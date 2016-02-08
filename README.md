Dendron
=======

Dendron is a [Matrix](https://matrix.org "Matrix") homeserver written in Go.

Rather than write a Matrix homeserver from scratch, Dendron acts as a proxy for
an existing homeserver [Synapse](https://github.com/matrix-org/synapse "Synapse")
written in python. This means that it can be already be used as a fully-featured
homeserver.

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

You will need to configure Synapse to use [PostgreSQL])https://github.com/matrix-org/synapse#using-postgresql)
for storage.

You will need to setup an unsecure HTTP listener for Dendron to proxy requests to.

### Configuring Dendron

The configuration for Dendron is passed on the command line.


SyTest
------

Dendron can be tested using [SyTest](https://github.com/matrix-org/sytest#dendron), a 
black box matrix integration tester. See the SyTest project page for instructions
