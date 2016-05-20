# Cloud Foundry CDN Service Broker [![Build Status](https://travis-ci.org/18F/cf-cdn-service-broker.svg?branch=master)](https://travis-ci.org/18F/cf-cdn-service-broker)

A [Cloud Foundry](https://www.cloudfoundry.org/) [service broker](http://docs.cloudfoundry.org/services/) for [CloudFront](https://aws.amazon.com/cloudfront/) and [Let's Encrypt](https://letsencrypt.org/).

## Deployment

### Automated

The easiest/recommended way to deploy the broker is via the [Concourse](http://concourse.ci/) pipeline.

1. Create a `ci/credentials.yml` file, and fill in the templated values from [the pipeline](ci/pipeline.yml).
1. Deploy the pipeline.

    ```bash
    fly -t lite set-pipeline -n -c ci/pipeline.yml -p deploy-cdn-broker -l ci/credentials.yml
    ```

### Manual

1. Clone this repository, and `cd` into it.
1. Target the space you want to deploy the broker to.

    ```bash
    $ cf target -o <org> -s <space>
    ```

1. Set the `environment_variables` listed in [the deploy pipeline](ci/pipeline.yml).
1. Deploy the broker as an application.

    ```bash
    $ cf push
    ```

1. [Register the broker](http://docs.cloudfoundry.org/services/managing-service-brokers.html#register-broker).

    ```bash
    $ cf create-service-broker cdn-route [username] [password] [app-url] --space-scoped
    ```

## Usage

1. Target the space your application is running in.

    ```bash
    $ cf target -o <org> -s <space>
    ```

1. Create a service instance.

    ```
    $ cf create-service cdn-route cdn-route my-cdn-route \
        -c '{"domain": "my.domain.gov", "origin": "my-app.apps.cloud.gov"}'

    Create in progress. Use 'cf services' or 'cf service my-cdn-route' to check operation status.
    ```

1. Get the DNS instructions.

    ```
    $ cf service my-cdn-route

    Last Operation
    Status: create in progress
    Message: Provisioning in progress; CNAME domain "my.domain.gov" to "d3kajwa62y9xrp.cloudfront.net."
    ```

1. Create/update your DNS configuration.
1. Wait thirty minutes for the CloudFront distribution to be provisioned and the DNS changes to propagate.
1. Visit `my.domain.gov`, and see that you have a valid certificate (i.e. that visiting your site in a modern browser doesn't give you a certificate warning).

## Tests

```
go test -v $(go list ./... | grep -v /vendor/)
```

## Contributing

See [CONTRIBUTING](CONTRIBUTING.md) for additional information.

## Public domain

This project is in the worldwide [public domain](LICENSE.md). As stated in [CONTRIBUTING](CONTRIBUTING.md):

> This project is in the public domain within the United States, and copyright and related rights in the work worldwide are waived through the [CC0 1.0 Universal public domain dedication](https://creativecommons.org/publicdomain/zero/1.0/).
>
> All contributions to this project will be released under the CC0 dedication. By submitting a pull request, you are agreeing to comply with this waiver of copyright interest.
