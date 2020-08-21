# Metal Console Architecture

`metal-console` is used to provide console access to all metal machines via SSH.
It is designed to serve also as an out-of-band (OOB) console if an proper admin private SSH key is used.

## High level design

`metal-console` is composed of four main components.

The first one, called `metal-console`, is an SSH server listening on port `5222` running on the metal-control-plane.
It accepts SSH requests with machine IDs as user and opens an SSH connection to the corresponding machine.
Example usage:

```bash
ssh -p 5222 <machine-ID>@<mgmt-host>
```

First, it fetches the corresponding IPMI data and SSH public key(s) from `metal-api` for the requested machine as well as the address of the management servers that are responsible for all machines of the requested machines partition. It then opens a connection to one of these management servers and copies all stdin, stdout and stderr traffic into both directions.

The connection is accepted by a the third component, the `bmc-reverse-proxy`, which is a simple Nginx running on each management server. It provides a secured (client certificate) pipe to the last component, the `bmc-proxy`, which finally opens a connection to the requested machine.

## Traffic sequence

```bash
User <---> metal-console <---> bmc-reverse-proxy <---> bmc-proxy <---> machine
                               |_______management-server_______|
```
