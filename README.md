# Cloud Foundry CDN Service Broker

A [Cloud Foundry](https://www.cloudfoundry.org/) [service broker](http://docs.cloudfoundry.org/services/) for [CloudFront](https://aws.amazon.com/cloudfront/) and [Let's Encrypt](https://letsencrypt.org/).

## Deployment

1. Clone this repository, and `cd` into it
1. Target the space you want to deploy the broker to

    ```bash
    $ cf target -o <org> -s <space>
    ```

1. Create a [user-provided service](http://docs.cloudfoundry.org/devguide/services/user-provided.html) with the configuration (you will be prompted for the values of each)

    ```bash
    $ cf create-service rds micro-psql cdn-rds
    $ cf create-user-provided-service cdn-creds -p 'BROKER_USER,BROKER_PASS,DATABASE_URL,EMAIL,ACME_URL,BUCKET,AWS_ACCESS_KEY_ID,AWS_SECRET_ACCESS_KEY,AWS_DEFAULT_REGION'
    ```

1. Deploy the broker as an application

    ```bash
    $ cf push
    ```

1. [Register the broker](http://docs.cloudfoundry.org/services/managing-service-brokers.html#register-broker)

    ```bash
    $ cf create-service-broker cdn-route [username] [password] [app-url] --space-scoped
    ```

## Usage

1. Target the space your application is running in

    ```bash
    $ cf target -o <org> -s <space>
    ```

1. Create service instance

    ```bash
    $ cf create-service cdn-route cdn-route my-cdn-route \
        -c '{"domain": "my.domain.gov", "origin": "my-app.apps.cloud.gov"}'

    Create in progress. Use 'cf services' or 'cf service my-cdn-route' to check operation status.
    ```

1. Get CNAME instructions

    ```bash
    $ cf service my-cdn-route

    Last Operation
    Status: create in progress
    Message: Provisioning in progress; CNAME domain "my.domain.gov" to "d3kajwa62y9xrp.cloudfront.net."
    ```

1. Create/update your CNAME in your DNS configuration
1. Create a [private domain](http://docs.cloudfoundry.org/devguide/deploy-apps/routes-domains.html#private-domains) in your Cloud Foundry organization

    ```bash
    $ cf create-domain <org> <domain>
    ```

1. Add the following under the application entry in your [manifest](https://docs.cloudfoundry.org/devguide/deploy-apps/manifest.html)

    ```yaml
    domain: <domain>
    no-hostname: true
    ```

1. Wait thirty minutes for the CloudFront distribution to be provisioned and the DNS changes to propagate.
1. Visit `my.domain.gov`, and see that you have a valid certificate (i.e. that visiting your site in a modern browser doesn't give you a certificate warning)

## Contributing

See [CONTRIBUTING](CONTRIBUTING.md) for additional information.

## Public domain

This project is in the worldwide [public domain](LICENSE.md). As stated in [CONTRIBUTING](CONTRIBUTING.md):

> This project is in the public domain within the United States, and copyright and related rights in the work worldwide are waived through the [CC0 1.0 Universal public domain dedication](https://creativecommons.org/publicdomain/zero/1.0/).
>
> All contributions to this project will be released under the CC0 dedication. By submitting a pull request, you are agreeing to comply with this waiver of copyright interest.
