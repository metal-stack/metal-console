# metal-console

`metal-console` provides access to the serial console of metal machines.
This is achieved by acting as a bridge between ssh and the console protocol of the concrete machine.
It will support either libvirt based console access, which is used in the development environment.
On real hardware ipmi based lanplus sol (Serial Over LAN) will be used.

To access the console execute:

```bash
ssh -i <private key> <uuid of the machine>@<hostname of metal-console server>
```

The metal-console will then lookup the given username as machine uuid on metal-api, request which console protocol to use.
If the machine uuid is a valid machine, it will then use the provided private key to authenticate against the ssh public key stored in the metal-api for this machine. If access is granted, the user will have access to the console.

`metal-console` figures out in which partition the machine is located and then opens a tls socket connection to `metal-bmc` running on the management server in this partition. `metal-bmc` checks if the tls client certificate matches. If this is the case, it looks up the machine ipmi details from `metal-api` and starts a ipmi sol session to the machine.

## TODO

- If a second console access starts to same machine, kill existing one
