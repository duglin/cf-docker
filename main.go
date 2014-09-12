package main

import (
    "bytes"
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "strconv"
    "strings"
    "sync"
    "time"
)

var containers = make( map[string]time.Time )   // todo: semaphore
var verbose    = false
var port       = 9999 
var justBuild  = false

var dockerHost    string
var dockerMonitor string

func debug(v ...interface{}) bool {
    if verbose && v != nil { 
        log.Println( v... )
    }
    return verbose
}

func main() {
    flag.IntVar( &port, "p", 9999, "Listen port number" )
    flag.BoolVar( &verbose, "v", false, "Turn on debugging output" )
    flag.BoolVar( &justBuild, "b", false, "Just do docker build step" )
    flag.Parse()

    dockerHost    = os.Getenv( "DOCKER_HOST" )
    dockerMonitor = os.Getenv( "DOCKER_MONITOR" )

    debug( "DOCKER_HOST=" + dockerHost )
    debug( "DOCKER_MONITOR=" + dockerMonitor )

    if dockerHost == "" {
      fmt.Println( "DOCKER_HOST must be set" );
      os.Exit( 1 )
    }

    if os.Getenv( "VCAP_APPLICATION" ) != "" {
        // Running in CF
        cid := StartDockerContainer()
        if !justBuild {
          go RegisterThread( cid )
          StartProxy( cid )
        }
    } else {
        // Running the container monitor
        go DeleteThread()
        StartListener()
    }
}

func StartDockerContainer() string {
    if ePort := os.Getenv( "VCAP_APP_PORT" ); ePort != "" {
        port,_ = strconv.Atoi( ePort )
    }

    var data     map[string]interface{} // string
    var imgName  string 
    var out      bytes.Buffer

    vcapApp := os.Getenv( "VCAP_APPLICATION" )
    json.Unmarshal( []byte(vcapApp), &data )
    appID := data["version"].(string)

    // Discover the image name or build a new image 
    if imgBytes,err := ioutil.ReadFile( "Dockerimage" ); err == nil {
        imgName = strings.TrimSpace( string(imgBytes) )
    } else {
        imgName = appID
        Exec( "docker", "build", "-t", imgName, "." )
        ioutil.WriteFile( "Dockerimage", []byte(imgName), 0600 )
    }
    debug( "ImageName:", imgName )

    if justBuild { return "" }

    pOpt := "--publish-all=true"

    // Very special case - and only works when you have one instance
    if os.Getenv( "DOCKER_HOST_PORT" ) != "" {
        var cPort string

        data,_  = getImageInfo( imgName )
        cc     := data["ContainerConfig"].(map[string]interface{})
        ep     := cc["ExposedPorts"].(map[string]interface{})

        for k,_ := range ep {
            cPort = strings.Split( k, "/" )[0]
            break 
        }
    
        if cPort != "" {
          pOpt = "--publish=" + os.Getenv( "DOCKER_HOST_PORT" ) + ":" + cPort
        }
    }
 
    // Run it!
    cid,_ := Exec( "docker", "run", "-d", "--env-file", "../env.lst", 
                   pOpt, imgName )
    cid   = strings.TrimSpace( cid )
    debug( "CID:", cid )

    // Save container info to "docker.info" for retrieval via 'gcf files'
    data,_      = getContainerInfo( cid )
    contents,_ := json.Marshal( data )
    json.Indent( &out, contents, "", "  ")
    ioutil.WriteFile( "../docker.info", out.Bytes(), 0600 )
    return cid
}

func StartListener() {
    debug( "Listening on port:", port )
    portStr := strconv.Itoa( port )
    http.HandleFunc( "/register", doRegister ) 
    log.Println( http.ListenAndServe("0.0.0.0:"+portStr, nil) )
}

func StartProxy(cid string) {
    data,_ := getContainerInfo( cid )
    nets   := data["NetworkSettings"].(map[string]interface{})
    ports  := nets["Ports"].(map[string]interface{})

    var host     string
    var hostPort string 

    for k,v := range ports {
      k = strings.Split( k, "/" )[0]
      hostPort = v.([]interface{})[0].(map[string]interface{})["HostPort"].(string)
      break ;
    }

    portStr := strconv.Itoa( port )
    dURL,_  := url.Parse( dockerHost )
    host = strings.Split( dURL.Host, ":" )[0]

    debug( "Starting proxy on port:", port, " -> ", host+":"+hostPort )

    listener,_ := net.Listen( "tcp", "0.0.0.0:"+portStr )
    defer listener.Close()
    for {
        inConn,_ := listener.Accept()
        debug( "Got an incoming request", inConn )
        go func() {
            defer inConn.Close()
            outConn,err := net.Dial( "tcp", host + ":" + hostPort )
            if err != nil {
                debug( "Error:", err )
                return 
            }
            defer outConn.Close()

            var w sync.WaitGroup
            w.Add(2)
            go func() { defer w.Done(); io.Copy( inConn, outConn ) }()
            go func() { defer w.Done(); io.Copy( outConn, inConn ) }()
            w.Wait()
        }()
    }
}

func DeleteThread() {
    debug( "Starting container deletion thread..." )
    for {
        delay,_ := time.ParseDuration( "-9s" )
        cutOff    := time.Now().Add( delay )

        for cid,t := range containers {
            if t.Before( cutOff ) {
                debug( "Removing:", cid, "(", len(containers)-1, ")" )
                Exec( "docker", "rm", "-f", cid )
                delete( containers, cid )
            }
        }

        time.Sleep( 1*time.Second )
    }
}

func RegisterThread(cid string) {
    debug( "Starting container registration thread" )
    for { 
        //debug( "Registering to:", dockerMonitor + "/register?cid=" + cid )
        resp,err := http.Get( dockerMonitor + "/register?cid=" + cid )

        if resp.StatusCode == 410 {
          // Docker container is gone, so die to recreate it
          os.Exit( -1 )
        }

        if err != nil {
            debug( "resp:", resp, "\nerr:", err )
            if resp != nil {
                contents,_ := ioutil.ReadAll(resp.Body)
                debug( "contents:", contents );
            }
        }

        time.Sleep( 3*time.Second )
    }
}

func doRegister(w http.ResponseWriter, req *http.Request) {
    params,_ := url.ParseQuery( req.URL.RawQuery )
    cid      := params.Get( "cid" )

    data,_ := getContainerInfo( cid )
    if data == nil {
        debug( "Can't find container", cid, "so killing CF app" )
        delete( containers, cid )
        w.WriteHeader( http.StatusGone )
        return 
    }

    if containers[cid].IsZero() {
      debug( "Registering:", cid, "(", len(containers)+1, ")" )
    }
    containers[cid] = time.Now()
    w.WriteHeader( http.StatusOK )
}

func doDefault(w http.ResponseWriter, req *http.Request) {
    w.WriteHeader( http.StatusOK )
}

func getContainerInfo(cid string) (map[string]interface{},error) {
    tmpHost := strings.Replace( dockerHost, "tcp://", "http://", 1 )
    return getJSONfromURL( tmpHost + "/containers/" + cid + "/json" )
}

func getImageInfo(iid string) (map[string]interface{},error) {
    tmpHost := strings.Replace( dockerHost, "tcp://", "http://", 1 )
    return getJSONfromURL( tmpHost + "/images/" + iid + "/json" )
}

func getJSONfromURL(daURL string) (map[string]interface{},error) {
    var data     map[string]interface{}
    var contents []byte

    resp,err := http.Get( daURL )

    if resp.StatusCode == 404 { return nil, err }

    if err != nil {
        debug( "GET " + daURL )
        debug( "GET ERR: ", resp, "->", err )
    }
    if resp != nil {
        contents,_ = ioutil.ReadAll(resp.Body)
        json.Unmarshal( []byte(contents), &data )
    } else {
        return nil, errors.New( "Can't get image info" )
    }

    return data, nil
}

func Exec(args ...string) (string, error) {
    var out bytes.Buffer
    var errout bytes.Buffer

    cmd := exec.Command(args[0], args[1:]...)
    cmd.Stdout = &out
    cmd.Stderr = &errout

    err := cmd.Run()

    if err != nil {
        debug( "Exec: ", args );
        debug( "Exec.out:", strings.TrimSpace(out.String()) )
        debug( "Exec.err:", strings.TrimSpace(errout.String()) )
    }

    return out.String(), nil
}
