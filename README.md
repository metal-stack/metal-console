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

## Configuration

The `metal-console` can be configured through environment variables. 
Every configuration needs to be prefixed with `METAL_CONSOLE_`.

All configuration options can be found in the implementation [internal/console/spec.go](./internal/console/spec.go).

### Token Renewal

Once a valid `metal-apiserver` token is provided, `metal-console` is capable to renew it on its own when running in Kubernetes.

While running, the `metal-console` will update the referenced secret. Set the following environment variables:

- `METAL_CONSOLE_TOKEN_RENEWAL_PERSISTENCE=true`
- `METAL_CONSOLE_TOKEN_RENEWAL_NAMESPACE`
- `METAL_CONSOLE_TOKEN_RENEWAL_SECRET_NAME`
- `METAL_CONSOLE_TOKEN_RENEWAL_SECRET_KEY`

Also make sure to configure the service account accordingly.

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: metal-console
  namespace: metal-stack
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: metal-console-token-renewal
  namespace: metal-stack
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["metal-console-token"]
  verbs: ["get", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: metal-console-token-renewal
  namespace: metal-stack
subjects:
- kind: ServiceAccount
  name: metal-console
  namespace: metal-stack
roleRef:
  kind: Role
  name: metal-console-token-renewal
  apiGroup: rbac.authorization.k8s.io
```
