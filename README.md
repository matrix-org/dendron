Dendron
=======

Dendron was an experimental [Matrix](https://matrix.org "Matrix") homeserver written in Go.

Rather than write a Matrix homeserver from scratch, Dendron acted as a proxy for
an existing homeserver [Synapse](https://github.com/matrix-org/synapse "Synapse")
written in python. This meant that it could be used as a fully-featured
homeserver.

### The Dendron experiment mainly served to give us an excuse to learn Go and reveal that a strangler-pattern style rewrite like this isn't a clear win.  The additional hop of the dendron loadbalancer/proxy caused operational complexity (additional timeouts; logging; IP rewrites; FD limits; etc) whilst buying nothing that a normal LB couldn't do.  Also, the strangler-pattern didn't give us enough freedom to rapidly fix the problems of Synapse's DB schema and storage layer.

### As of March 9th 2017, matrix.org's Synapse is no longer running behind Dendron (but instead using a normal haproxy), and meanwhile all our next-generation homeserver development effort is going into [Dendrite](https://github.com/matrix-org/dendrite) (aka "Dendron done right" ;D), which in turn is built on [gomatrixserverlib](https://github.com/matrix-org/gomatrixserverlib), [matrix.org/util](https://github.com/matrix-org/util), and other libs.

### As such, Dendron development is dead, succeeded by Dendrite.

Building
--------

Currently to use Dendron you will need an development install of Synapse. See
[these instructions](https://github.com/matrix-org/synapse#synapse-development "Synapse Development") 
for setting up a dev install of Synapse.

To build Dendron itself check out this repository and build it using [gb](https://getgb.io):

    gb generate
    gb build


Configuring
-----------

WARNING: Dendron is currently in the early stages of development. These
instructions are likely to change frequently and no effort is made to provide
backwards compatibility.

### Configuring Synapse

You will need to configure Synapse to use [PostgreSQL](https://github.com/matrix-org/synapse#using-postgresql)
for storage.

You will need to setup an unsecure HTTP listener for Dendron to proxy requests to.

### Configuring Dendron

The configuration for Dendron is passed on the command line.


SyTest
------

Dendron can be tested using [SyTest](https://github.com/matrix-org/sytest#dendron), a 
black box matrix integration tester. See the SyTest project page for instructions
