# Installing the VNI service stack

Assumptions: A running kubernetes cluster with the CXI CNI plugin deployed.


## VNI CRD and Controller
First, install Metacontroller. Refer to the Metacontroller [documentation](https://metacontroller.github.io/metacontroller/guide/install.html) for information on how to install the Metacontroller.
Apply the file `config/metacontroller.yaml`, which contains some tuning options for the Metacontroller engine.

Create a namespace using file `config/vni-management-namespace.yml`.
The VNI CRD and controller can be installed using the yaml files `config/vni-crd.yml`, `config/vni-claim-crd.yml` and `config/vni-controller.yml`. 
Apply then e.g. via `kubectl`. 

By design, the VNI controller listens to resource creation events and acts upon those matching the configuration in `config/vni-controller.yml`.
As of now, Deployments, DaemonSets, ReplicaSets, Jobs, and volcano.sh-Jobs are configured. 
Adapt the configuration if you want to add support for other deployments!

## VNI Database & Endpoint

The VNI database is a sqlite3 file, which is automatically created. The endpoint is deployed as a Service.

First, build the endpoint in folder `endpoint/` by running:
```shell
go build \
  -ldflags '-linkmode external -s -w -extldflags "--static"' \
  -tags 'osusergo,netgo,static_build'
```
The result is a static binary.

Second, build a container image using the provided Containerfile, e.g. using
```shell
buildah build -t vni_service_endpoint -f endpoint/Containerfile
```

Upload it to your container registry of choice.

Finally, run the `vni-endpoint-deployment.yml` file, which should deploy the VNI Endpoint.
Make sure to adapt the image url to point to the image of your container registry of choice.

## Smarter Device Manager Deployment

Applications that want to use Slingshot need to have access to the `/dev/cxi*` device(s). 
In order to expose these to pods, you can use the Smarter Device Manager [1]. Install via the guide of that tool.

Next, add the following ConfigMap: 

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: smarter-device-manager
  namespace: device-manager
data:
  conf.yaml: |
    - devicematch: ^cxi0$
      nummaxdevices: 200
```
Adjust `devicematch` and `nummaxdevices` if necessary. `devicematch` supports regex for matching multiple CXI NICs.
All nodes with a CXI NIC in the cluster should now list the `smarter-devices/cxi0` resource.

Applications requiring Slingshot must now add the following lines to their description:

```yaml
resources:
    requests:
      smarter-devices/cxi0: "1"
    limits:
      smarter-devices/cxi0: "1"
```

[1] https://github.com/smarter-project/smarter-device-manager

## Usage

Attach the annotation `vni: true` to a Job you want a new VNI for. Alternatively, annotate with `vni: 'claim-name'` after
having created a VniClaim object. See `config/tests/vni-claim.yml` for an example VniClaim.
