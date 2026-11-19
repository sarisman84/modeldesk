# Local model library

The library describes the model files available on one machine and the ways a user wants to run them.

## Language

**Model files**: The local weights and associated files needed to run a particular model variant. Multiple profiles can share the same files.
_Avoid_: Profile, running model

**Profile**: A named configuration for running a model using a particular set of model files. Removing a profile keeps those files on disk.
_Avoid_: Download, copy of a model

**Import**: Registering existing model files in the library at their current location, without making a copy.
_Avoid_: Download
