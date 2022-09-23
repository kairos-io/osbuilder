# osbuilder

| :exclamation: | This is experimental! |
|-|:-|

This is the Kairos osbuilder Kubernetes Native Extension. 

To install, use helm:

```
# Adds the kairos repo to helm
$ helm repo add kairos https://kairos-io.github.io/helm-charts
"kairos" has been added to your repositories
$ helm repo update                                        
Hang tight while we grab the latest from your chart repositories...
...Successfully got an update from the "kairos" chart repository
Update Complete. ⎈Happy Helming!⎈

# Install the CRD chart
$ helm install kairos-crd kairos/kairos-crds
NAME: kairos-crd
LAST DEPLOYED: Tue Sep  6 20:35:34 2022
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None

# Installs osbuilder
$ helm install kairos-osbuilder kairos/osbuilder
```
