# cf-cdn-service-broker

A Cloud Foundry service broker for CloudFront and Let's Encrypt

## Deployment

* Create services

    Note: the following credentials must be available in the `cdn-creds` service or in top-level environment variables:

    * PORT
    * BROKER_USER
    * BROKER_PASS
    * DATABASE_URL
    * EMAIL
    * ACME_URL
    * BUCKET
    * AWS_ACCESS_KEY_ID
    * AWS_SECRET_ACCESS_KEY
    * AWS_DEFAULT_REGION

    ```
    $ cf create-service rds micro-psql cdn-rds
    $ cf create-user-provided-service cdn-creds -p '{"AWS_ACCESS_KEY_ID": "[key-id]", ...}'
    ```

* Deploy application

    ```
    $ GOOS=linux GOARCH=amd64 go build . && cf push
    ```

* Add to Cloud Foundry

    ```
    $ cf create-service-broker cdn-route [username] [password] [app-url] --space-scoped
    ```

## Usage

* Create service

    ```
    $ cf create-service cdn-route cdn-route my-cdn-route \
        -c '{"domain": "my.domain.gov", "origin": "my-app.apps.cloud.gov"}'

    Create in progress. Use 'cf services' or 'cf service my-cdn-route' to check operation status.
    ```

* Get CNAME instructions

    ```
    $ cf service my-cdn-route

    Last Operation
    Status: create in progress
    Message: Provisioning in progress; CNAME domain "my.domain.gov" to "d3kajwa62y9xrp.cloudfront.net."
    ```

* Update CNAME

* Wait for changes to propagate (may take 30 minutes)

* Visit `my.domain.gov`

## Contributing

See [CONTRIBUTING](CONTRIBUTING.md) for additional information.

## Public domain

This project is in the worldwide [public domain](LICENSE.md). As stated in [CONTRIBUTING](CONTRIBUTING.md):

> This project is in the public domain within the United States, and copyright and related rights in the work worldwide are waived through the [CC0 1.0 Universal public domain dedication](https://creativecommons.org/publicdomain/zero/1.0/).
>
> All contributions to this project will be released under the CC0 dedication. By submitting a pull request, you are agreeing to comply with this waiver of copyright interest.
