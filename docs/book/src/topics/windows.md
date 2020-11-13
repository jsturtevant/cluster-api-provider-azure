# Windows clusters

## Overview

CAPZ enables you to create Windows Kubernetes clusters on Microsoft Azure.

To deploy a cluster using Windows, use the [Windows flavor template](https://raw.githubusercontent.com/kubernetes-sigs/cluster-api-provider-azure/master/templates/cluster-template-windows.yaml).

## Deploy a work load

After you Windows VM is up and running you can deploy a workload. Using the deployment file below:

```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: iis-1809
  labels:
    app: iis-1809
spec:
  replicas: 1
  template:
    metadata:
      name: iis-1809
      labels:
        app: iis-1809
    spec:
      containers:
      - name: iis
        image: mcr.microsoft.com/windows/servercore/iis:windowsservercore-ltsc2019
        resources:
          limits:
            cpu: 1
            memory: 800m
          requests:
            cpu: .1
            memory: 300m
        ports:
          - containerPort: 80
      nodeSelector:
        "kubernetes.io/os": windows
  selector:
    matchLabels:
      app: iis-1809
---
apiVersion: v1
kind: Service
metadata:
  name: iis
spec:
  type: LoadBalancer
  ports:
  - protocol: TCP
    port: 80
  selector:
    app: iis-1809
```

Save this file to iis.yaml then deploy it:

```
kubectl apply -f .\iis.yaml
```

Get the Service endpoint and curl the website:

```
kubectl get services
NAME         TYPE           CLUSTER-IP   EXTERNAL-IP   PORT(S)        AGE
iis          LoadBalancer   10.0.9.47    <pending>     80:31240/TCP   1m
kubernetes   ClusterIP      10.0.0.1     <none>        443/TCP        46m


curl <EXTERNAL-IP>
```

## Details

See the CAPI proposal for implementation details: https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20200804-windows-support.md

### Image creation
The images are build using image-builder [Windows support](https://github.com/kubernetes-sigs/image-builder/pull/382) 
and use [Cloudbase-init](https://cloudbase-init.readthedocs.io/en/latest/) to bootstrap the machines.  

### VM password and access
The VM password is [random generated](https://cloudbase-init.readthedocs.io/en/latest/plugins.html#setting-password-main)
by Cloudbase-init during provisioning of the VM. For Access to the VM you can use ssh which will be configured with SSH
public key you provided during deployment. 

To SSH:

```
ssh -t -i .sshkey -o 'ProxyCommand ssh -i .sshkey -W %h:%p capi@<api-server-ip>' capi@<windows-ip> powershell.exe
```

To RDP:

```
ssh -L 5555:10.1.0.4:3389 capi@20.69.66.232
```

And then open an RDP client to `localhost:5555`

### Kube-proxy and CNI's

Kube-proxy and Windows CNI's are deployed via Cluster Resource Sets.  Windows doesn't not have a kube-proxy image due 
to not having Privileged containers which would provide access to the host.  The current solution is using wins.exe as 
demonstrated in the [Kubeadm support for Windows](https://kubernetes.io/docs/tasks/administer-cluster/kubeadm/adding-windows-nodes/).    

Windows Privileged Container support is in [KEP](https://github.com/kubernetes/enhancements/pull/2037) form with plans to 
implement in 1.21.  Kube-proxy and other CNI will then be replaced with the Privileged containers. 




