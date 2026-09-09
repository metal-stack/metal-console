# Metal Console Architecture

`metal-console` is used to provide console access to all metal machines via SSH.
It is designed to serve also as an out-of-band (OOB) console.

## High level design

The `metal-console` is an SSH server listening on port `2222` running in the metal-control-plane.
It accepts SSH requests with machine IDs as user and opens an SSH connection to the corresponding machine.
Example usage:

```bash
ssh -p 2222 <machine-ID>@<metal-console-tcp-route>
```

It fetches the corresponding SSH public key(s) from `metal-apiserver` for the requested machine as well as the address of the management servers that are responsible for all machines of the requested machines partition. It then opens a connection to one of these management servers and forwards all stdin, stdout and stderr traffic into both directions.

The connection is accepted by the [metal-bmc](https://github.com/metal-stack/metal-bmc) running on each management server. It provides a secured (client certificate) and opens a connection to the requested machine.

## Traffic sequence

```bash
User <---> metal-console <---> metal-bmc <---> machine
```
