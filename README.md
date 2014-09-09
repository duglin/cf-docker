Cloud Foundry Docker Buildpack
==============================

The Cloud Foundry Docker Buildpack is a buildpack that allows you to deploy Docker applications via the Cloud Foundry interface. This is just a proof-of-concept and therefore has some limitations but does show what the basic user experience might be like if Cloud Foundry supported Docker in a first class way.

The buildpack requires that you have a Docker host available for it to access, and you need to also have a Docker container manager app running that will sync your Cloud Foundry runtime with the Docker containers.  If the Docker container manager app is not running then containers will still be created, but they will not be deleted as your app is stopped or scaled down.

Installation
------------
You'll need to do the following:
* Have a Cloud Foundry installed - no changes are needed for this to work
* Deploy the `docker` buildpack that comes with this buildpack. It is easiest to just do a `cf create-buildpack ...` pointing to the zip file in the root of this repo:  http://    It assumes your DEAs are running Ubuntu.
* Have a Docker host available for the buildpack to use and accessible from the Cloud Foundry DEAs/apps.
* Run the `cf-docker` application in this repo.  You can `go` compile it - its just a single file. The Mac binary is here and the Ubuntu binary is here - if you can use those.  Ensure that you have the `DOCKER_HOST` environment variable set to the URL it needs to use to talk to your Docker host. The `docker` executable must also be available in this environment - hopefully this requirement will be removed later. Note: the `cf-docker` app must be accessible to the DEAs/apps in your Cloud Foundry.
```
export DOCKER_HOST=tcp://mydocker:2375
./cf-docker -p 9999
````
Will start `cf-docker` listening on port 9999.

Usage
-----
There are two different types of Docker applications that can be deployed: Docker images and Dockerfile-based apps.

In both cases the buildpack will require two bits of information in order to work - both passed to it via environment variables:
* DOCKER_HOST - the URL to the Docker host on which the applications will be built and deployed
* DOCKER_MONITOR - the URL to the Docker container manager.

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

You can see what the Docker host's mapped ports are for your app by examining the `app/docker.info` file in your Cloud Foundry app:  `cf files myapp app/docker.info`.

To see it in action try going to the `mysql` directory:
```
# edit the manifest file to set the DOCKER* environment variables
export DOCKER_HOST=...
cf push myql
docker ps     # to see the one new container
cf scale -i 5 mysql
docker ps     # to see 4 new containers
cf scale -i 1 mysql
docker ps     # back down to one container
cf stop mysql
docker ps     # all containers gone now


Dockerfile-Based Apps
---------------------
Simply `cf push` your Docker app to Cloud Foundry, ensuring that there is a Dockefile in the root of the app. So, basically, do a `cf push` in place of a `docker build ...` and a `docker run ...`.

Docker Image-based Apps
-----------------------
To ask the buildpack to create a new Docker container based on an existing Docker image, simply create a file called `Dockerimage` in the root of your app's directory and place the name of the image in there. Most likely your app will just consist of two file - the `Dockerimage` file and the `manifest.yml` to set the DOCKER_HOST and DOCKER_MONITOR environment variables.

Notes
=====
Dockfiles that do lots of work may have issues due to the amount of time it'll take for the Docker containers to be built.  To help this along, it would be best if you pre-built them via `docker build` just so things are cached.  Remember, this is just a PoC and if Cloud Foundry supports Docker in a first class way then this can be fixed.

Your Cloud Foundry environment variables (including VCAP_SERVICES) should be available to your Docker containers.

More to come, I'm sure more info is needed

TODOs & Limitations
===================
* Support accessing Docker via REST calls instead of the cmd line - too little time :-)
* Detect when a container is gone and kill the CF app so HM9000 will kick in
* Support multiple Docker hosts 
