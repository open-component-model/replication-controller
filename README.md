# replication-controller

The `replication-controller` is part of the Open Component Model Kubernetes controller set that enables transferring components from one OCM repository to another.

The behaviour of the `replication-controller` is similar to that of the `ocm transfer` command with the addition of a reconciliation loop. It can therefore be used to "subscribe" to components and ensure that any component versions matching a semantic version constraint will be replicated from the source OCM repository to the destination.

### Installation

Install the latest version of the controller using the following command:

```bash
VERSION=$(curl -sL https://api.github.com/repos/open-component-model/replication-controller/releases/latest | jq -r '.name')

kubectl apply -f https://github.com/open-component-model/replication-controller/releases/download/$VERSION/install.yaml
```

### Usage

```yaml
apiVersion: delivery.ocm.software/v1alpha1
kind: ComponentSubscription
metadata:
  name: podify-subscription
  namespace: ocm-system
spec:
  interval: 10m
  component: github.com/weaveworks/podify
  semver: "=>v1.0.0"
  source:
    url: ghcr.io/phoban01
    secretRef:
      name: creds
  destination:
    url: ghcr.io/$GITHUB_USER
    secretRef:
      name: creds
  verify:
  - signature:
      name: signature-name
      publicKey:
        secretRef:
          name: public-key-secret
```
