[![REUSE status](https://api.reuse.software/badge/github.com/open-component-model/replication-controller)](https://api.reuse.software/info/github.com/open-component-model/replication-controller)

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

## Contributing

Code contributions, feature requests, bug reports, and help requests are very welcome. Please refer to the [Contributing Guide in the Community repository](https://github.com/open-component-model/community/blob/main/CONTRIBUTING.md) for more information on how to contribute to OCM.

OCM follows the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md).

## Licensing

Copyright 2022-2023 SAP SE or an SAP affiliate company and Open Component Model contributors.
Please see our [LICENSE](LICENSE) for copyright and license information.
Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/open-component-model/replication-controller).
