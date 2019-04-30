# Metal Console Architecture

`metal-console` is used to provide console access to all metal machines via SSH.
It is designed to serve also as an out-of-band (OOB) console if an proper admin private SSH key is used.

## High level design

`metal-console` is composed of four main components.

The first one, called `metal-console`, is an SSH server listening on port `5222` running on the metal-control-plane.
It accepts SSH requests with machine IDs as user and opens an SSH connection to the corresponding machine.
Example usage:
```
ssh -p 5222 <machine-ID>@metal.test.fi-ts.io
```

First, it fetches the corresponding IPMI data and SSH public key(s) from `metal-api` for the requested machine as well as the address of the management service that is responsible for all machines of the requested machines partition. It then opens a connection to that management service and copies all stdin, stdout and stderr traffic to both directions.

The second component type is the management service mentioned above. There is one management service per partition running on the metal-control-plane (ReplicaSet), which is connected (and load-balances) to all management servers within that partition.
It therefore forwards the `metal-console` traffic to one of its management servers by connecting to the third component, the `bmc-reverse-proxy`, which is running on each management server.

The `bmc-reverse-proxy` is a simple Nginx server that provides a secured (client certificate) pipe to the last component, the `bmc-proxy`,
which finally opens a connection to the requested machine.

## Traffic sequence

```
User <---> metal-console <---> management-service <---> bmc-reverse-proxy <---> bmc-proxy <---> machine
           |_________metal-control-plane________|       |_______management-server_______|
```