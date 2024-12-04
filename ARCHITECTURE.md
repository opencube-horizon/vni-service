# VNI Service

This repository contains the code for the HPE Cray Slingshot 11 VNI Service for deployment in a Kubernetes cluster.

## Architecture

The VNI Service is responsible for handing out VNIs (Virtual Networ IDs) to requesting Kubernetes "jobs" and 
subsequently recycling them upon job termination. A "job" is considered a K8s resource instance which governs the lifetime 
of a set of pods. Examples of a job are (actual) Jobs, ReplicaSets or Deployments.

### Requirements
(1) A VNI may not be handed out to two unrelated jobs.
(2) VNIs must be uniquely allocated to the requesting job(s) for the entirety of their lifetime.
(3) VNIs must be recycled after the associated job has terminated.


### Components
The VNI Service consists of roughly three components:

1. VNI Custom Resource Definition (CRD)
2. VNI CRD Controller
3. VNI Database & Endpoint

#### VNI Custom Resource Definition

The VNI CRD defines the K8s-internal representation of VNIs. 
An instantiated VNI Resource object stores one VNI as allocated in the VNI Database (3), which is an integer.
It is associated to one job.
The association to jobs is achieved using ownership relations, i.e. a job "owns" the VNI.

VNI resource objects have the name `vni-<uid-of-owning-job>` and are namespaced to the namespace of the owning job.

The VNI CRD is defined in `configs/vni-crd.yml`.

#### VNI CRD Controller

VNI resource objects in and of themselves only store state, in this case the VNI integer.
In order to act upon creation / deletion of VNI resource objects or the owning job objects, a custom Controller is used.
To reduce implementation of unrelated logic, the Metacontroller [1] project is used, in specific a DecoratorController.

A DecoratorController listens for the creation of a list of preconfigured resource objects and "decorates" them by creating
custom resource objects and attaching them to these objects.
In this case, the VNI DecoratorController 
(1) listens for the creation of job objects such as Jobs or ReplicaSets,
(2) requests a new VNI from the VNI Database, and 
(3) creates a VNI resource object storing the requested VNI integer and attaches it to the triggering job.

The VNI DecoratorController is implemented using a simple yaml (`config/vni-controller.yml`) since the core logic
is already implemented via the Metacontroller project. The DecoratorController queries two webhooks `/sync` and `/finalize`, 
which are served by the VNI Database Endpoint. 

The `/sync` endpoint is called each time a change in the current Kubernetes configuration is detected. Such a change
can be a new VNI object being requested or a VNI object getting deleted. It is used to request a new VNI from the VNI 
Database or fetch an existing VNI if one exists for the calling job.

The semantics of the `/finalize` endpoint is a specialization of the `/sync` endpoint in that it is only called on 
events where the involved object should be deleted. When set, the involved VNI object is only deleted after the endpoint
has been called. It is used to inform the VNI Database that the VNI is no longer used and can be deleted.

#### VNI Database & Endpoint

As outlined in the previous section, the VNI CRD Controller calls the two endpoints `/sync` and `/finalize`, which are provided
by the VNI endpoint. The source code is stored in the `endpoint` folder.

##### Sync

The `/sync` endpoint is called with a list of "attachments", referring to the VNI resource objects to be created.
For each attachment, a new VNI is acquired from the database (see below).
All new objects are returned in the response body.

##### Finalize

Similar to the `/sync` endpoint, the `/finalize` endpoint is called with a list of attachments / VNI objects with are to be
deleted. For each attachment, the corresponding VNI is released from the database (see below).
An empty attachment list is returned, indicating that all VNIs have been released.


##### Database

The database is currently a sqlite3 file, since no replication is required.
The table `vni_allocs` stores the current VNI allocations and has the following schema: 

```sqlite
create table if not exists
    vni_allocs (
		uid string not null,
		namespace string not null, 
        vni integer not null,
        unique (uid, namespace, vni), 
        primary key (uid, namespace)
    );
```
The uniquely identifying key is the tuple (uid, namespace), where UID refers to the VNI name `vni-<uid-of-owning-job>`.

The Acquire function first checks whether a VNI is present in the database and returns it if so. If no VNI is present, 
a new VNI is generated in the range `[vniMin, vniMax)`. A similar approach to the one [taken](https://github.com/SchedMD/slurm/blob/7eecd351f679a2de9d0580149e35d95d6d6af7ed/src/plugins/switch/hpe_slingshot/config.c#L126)
in the Slingshot plugin in Slurm is taken: A bool list of size `vniMax-vniMin` is allocated where all entries are set to 
true where an VNI with that index is currently allocated. The next free VNI is the first non-true entry in that list.
Note that this table is currently generated for each `Acquire` call.

During Release, the corresponding entry in `vni_allocs` is deleted.

# Links

[1] https://metacontroller.github.io/metacontroller/