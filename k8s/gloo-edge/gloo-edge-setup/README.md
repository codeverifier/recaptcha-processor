```
helm pull gloo-ee/gloo-ee \
    --version=1.14.6 \
    --untar \
    --untardir ._output

kubectl apply -f ._output/gloo-ee/charts/gloo/crds
rm -rf ._output/gloo-ee

    helm upgrade --install gloo-ee gloo-ee/gloo-ee \
        --namespace gloo-system \
        --create-namespace \
        --version=1.14.6 \
        --set-string license_key=${GLOO_EDGE_LICENSE_KEY} \
        -f $helm_file
```