# reCAPTCHA Processor

This project is built for processing reCAPTCHA requests using Gloo Edge / Gloo Gateway (Kubernetes native API Gateway).
reCAPTCHA Processor service is running as an external authentication passthrough service in the API Gateway to authorize any requests.

Currently supports both reCAPTCHA and reCAPTCHA Enterprise.

This provides a [client](test/client/README.md) and a [backend service](test/backend-server/README.md) for running integration tests in a Kubernetes environment.

Refer to the diagram below for the flow of requests.

![reCAPTCHA Request Flow](https://github.com/pseudonator/recaptcha-processor/assets/2648624/63602e7d-a2dd-4d4b-9604-aa37c336d668)

## Building

- Use `make build` to build the processor and the test services.
- To build and push the Docker images use `PUSH_MULTIARCH=true make docker`. 
  By default it only builds `linux/amd64` & `linux/arm64` and the images get pushed to `australia-southeast1-docker.pkg.dev/field-engineering-apac/public-repo`.
- Run `make help` for all the build directives.

## Test

To test the recaptcha processor in a Kubernetes environment follow the steps below.

Follow the official Google docs to setup authentication for [non-Google environments](https://cloud.google.com/recaptcha-enterprise/docs/set-up-non-google-cloud-environments) and [Google environments](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity).

For the testing procedures below it uses the non-Google environment setup.

### Gloo Edge

1. Provision a cluster with Gloo Edge

    ```
    colima start --runtime containerd -p processor-test -c 4 -m 8 -d 20 --network-address --install-metallb --metallb-address-pool "192.168.106.230/29" --kubernetes --kubernetes-disable traefik,servicelb --kubernetes-version v1.24.13+k3s1

    helm upgrade --install gloo-ee gloo-ee/gloo-ee \
        --namespace gloo-system \
        --create-namespace \
        --version=1.14.6 \
        --set-string license_key=${GLOO_EDGE_LICENSE_KEY} \
        -f k8s/gloo-edge/gloo-edge-setup/gloo-edge-helm-values.yaml
    ```

2. Setup all the services

    ```
    kubectl create ns recaptcha
    # Step below depends on https://cloud.google.com/recaptcha-enterprise/docs/set-up-non-google-cloud-environments
    kubectl create secret generic -n recaptcha google-application-credentials --from-file=./application-credentials.json
    kubectl apply -f k8s/gloo-edge/processing-server/deployment.yaml -n recaptcha

    kubectl create ns apps
    kubectl apply -f k8s/gloo-edge/test-services/deployment.yaml -n apps
    kubectl apply -f k8s/gloo-edge/test-services/upstream.yaml -n gloo-system

    # Apply all the configs
    kubectl apply -f k8s/gloo-edge/config
    ```

### Gloo Gateway

TODO

## Future Improvements

- CI automatation to build Docker images
- Unit tests
- Migrate to using `nerdctl` instead of `docker`
