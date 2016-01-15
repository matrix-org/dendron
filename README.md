Dendron
=======

Dendron is a Matrix(link to matrix.org) homeserver written in go(lang?)
(what is this language called again?).

Rather than write a Matrix homeserver from scratch, Dendron acts as a proxy for
an existing homeserver synapse(link to synapse) written in python.
This means that it can be already be used as a fully-featured homeserver.

You can track our progress in implementing the Matrix APIs at (link to some kind of tracking thing?)

Building
--------

Currently to use Dendron you will need an working install of Synapse. Instructions
for setting up Synapse (link to synapse install instructions).

To build Dendron itself:

   (instructions for building a go thing)


Configuring Synapse
-------------------

You will need to configure Synapse with a plain HTTP listener so that Dendron can forward
requests to it:

    (instructions on setting up the listener)

Once Synapse is configured then Dendron can be started by running:

    (example command line for starting Dendron)

SyTest
------

Dendron can be tested using sytest(link to sytest), a black box matrix integration tester.
See the sytest project page for instructions(link to project page).
