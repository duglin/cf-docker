Cloud Foundry Docker Buildpack
==============================

The Cloud Foundry Docker Buildpack is a buildpack that allows you to deploy Docker applications to a Docker host via the Cloud Foundry interface. Any Docker application, not just HTTP webapps, can be deployed using this. While the application is hosted outside of the Cloud Foundry DEAs, the application management aspects of Cloud Foundry (e.g. health manager, scaling, env vars for service binding info) will still be used and available to your Docker-hosted application.

This is just a proof-of-concept and therefore has some limitations but does show what the basic user experience might be like if Cloud Foundry supported Docker in a first class way.

The buildpack requires that you have a Docker host available for it to access, and you need to also have a Docker container manager app (cf-docker) running that will sync your Cloud Foundry runtime with the Docker containers.  If the Docker container manager app is not running then containers will still be created, but they will not be deleted as your app is stopped or scaled down.

Installation
------------
You'll need to do the following:
* Have a Cloud Foundry installed - no changes are needed for this to work
* Deploy the `docker` buildpack that comes with this buildpack. It is easiest to just do a `cf create-buildpack docker https://github.com/duglin/cf-docker/raw/master/docker.zip`.  It assumes your DEAs are running Ubuntu.  You'll need to fork the repo and rebuild the zip/docker/cf-docker files in there if not.
* Have a Docker host available for the buildpack to use and accessible from the Cloud Foundry DEAs/apps.
* Run the `cf-docker` Docker container manager application in this repo.  You can `go` compile it - its just a single file (`go build -o cf-docker main.go`). The Mac binary is [here](https://github.com/duglin/cf-docker/raw/master/cf-docker) and the Ubuntu binary is [here](https://github.com/duglin/cf-docker/raw/master/buildpack/bin/cf-docker) - if you can use those.  Ensure that you have the `DOCKER_HOST` environment variable set to the URL it needs to use to talk to your Docker host. The `docker` executable must also be available in this environment - hopefully this requirement will be removed later. Note: the `cf-docker` app must be accessible to the DEAs/apps in your Cloud Foundry - so use an IP address/host that's visible to the DEAs.  To start the cf-docker to listen on port 9999 do:
```
export DOCKER_HOST=tcp://mydocker:2375
./cf-docker -p 9999
````

Usage
-----
There are two different types of Docker applications that can be deployed: Docker images and Dockerfile-based apps.

In both cases the buildpack will require two bits of information in order to work - both passed to it via environment variables:
* `DOCKER_HOST` - the URL to the Docker host on which the applications will be built and deployed
* `DOCKER_MONITOR` - the URL to the cf-docker Docker container manager.

The easiest way to set these is via a manifest file:
```
---
applications:
- name: myapp
  env:
      DOCKER_HOST: tcp://10.80.102.243:2375
      DOCKER_MONITOR: http://10.80.102.243:9999
```
But you can just as easily set them via `cf set-env` as well.

The Docker containers created will be setup to expose all ports defined by the `EXPOSE` command on the Docker host.  The Cloud Foundry runtime will expose just one of those ports via its runtime.  Meaning, if you deploy a webapp you should be able to access it via the same URL/route that Cloud Foundry associates with your app.  However, if you have other ports open, those are only accessible via the Docker host directly (as is the webapp's ports too).

You can see what the Docker host's mapped ports are for your app by examining the `app/docker.info` file in your Cloud Foundry app:  `cf files myapp app/docker.info` .

To see it in action try going to the `mysql` directory:
```
# edit the manifest file to set the DOCKER* environment variables
export DOCKER_HOST=...
cf push myql
docker ps             # to see the one new container
cf scale -i 5 mysql
docker ps             # to see 4 new containers
cf scale -i 1 mysql
docker ps             # back down to one container
cf stop mysql
docker ps             # all containers gone now
```

If you don't see the Docker container go away then make sure your cf-docker Docker container manager is running. Once started it can only manage containers that it sees from that point on, old ones will need to be killed manually.


Dockerfile-Based Apps
---------------------
Simply `cf push` your Docker app to Cloud Foundry, ensuring that there is a Dockefile in the root of the app. So, basically, do a `cf push` in place of a `docker build ...` and a `docker run ...`.  See the `mysql` directory for a sample.

Docker Image-based Apps
-----------------------
To ask the buildpack to create a new Docker container based on an existing Docker image, simply create a file called `Dockerimage` in the root of your app's directory and place the name of the image in there. Most likely your app will just consist of two files - the `Dockerimage` file and the `manifest.yml` to set the `DOCKER_HOST` and `DOCKER_MONITOR` environment variables.  See the `imagesql` directory for a sample.

Notes
=====
Dockfiles that do lots of work may have issues due to the amount of time it'll take for the Docker image to be built.  You may need to either increase the CF staging timeout or pre-build the image via a `docker build` so things are cached.

Your Cloud Foundry environment variables (including VCAP_SERVICES) should be available to your Docker containers.

How it works...
---------------
The `cf push` command will create a real CF app, but its not your app.  This app will spin up a new Docker container for each app instance and then act as a proxy for your app/container.

As the Cloud Foundry app instances come and go, so will the corresponding Docker container. Creation is easy, the app creates a new container. Delete is harder since the app may not get a chance to delete the container before it dies, so this is where the cf-docker Docker container manager comes in.  If it doesn't get a ping from an app instance within a few seconds it'll kill the corresponding Docker container.  This would be easy to do "properly" if CF managed the Docker containers directly.

The CF app created will act as a proxy and forward all incoming HTTP requests on to the Docker container on the first available port that is EXPOSEd.  So, normal webapps should work just fine.  Apps with more than one EXPOSEd port will be trickier since right now the CF app will just forward to one of them. Any other ports are available to end-users but only directly from the Docker host itself.

More to come, I'm sure more info is needed

Recent Changes
--------------
* When the Docker container dies the CF app will now be killed so HM9K can restart both

TODOs & Limitations
===================
* Support accessing Docker via REST calls instead of the cmd line so we can remove the embedded docker exe - too little time :-)
* Support multiple Docker hosts 
* Allow people to specify which EXPOSEd port to use for the CF app proxy
* Add ability to pass in Docker "run" options
* Run cf-docker as a docker app for them so they don't need to start it manually
* Interactions with Docker are not secured - yet.
