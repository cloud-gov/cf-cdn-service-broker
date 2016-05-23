## Welcome!

We're so glad you're thinking about contributing to an 18F open source project! If you're unsure about anything, just ask -- or submit the issue or pull request anyway. The worst that can happen is you'll be politely asked to change something. We love all friendly contributions.

We want to ensure a welcoming environment for all of our projects. Our staff follow the [18F Code of Conduct](https://github.com/18F/code-of-conduct/blob/master/code-of-conduct.md) and all contributors should do the same.

We encourage you to read this project's CONTRIBUTING policy (you are here), its [LICENSE](LICENSE.md), and its [README](README.md).

If you have any questions or want to read more, check out the [18F Open Source Policy GitHub repository]( https://github.com/18f/open-source-policy), or just [shoot us an email](mailto:18f@gsa.gov).

## Public domain

This project is in the public domain within the United States, and
copyright and related rights in the work worldwide are waived through
the [CC0 1.0 Universal public domain dedication](https://creativecommons.org/publicdomain/zero/1.0/).

All contributions to this project will be released under the CC0
dedication. By submitting a pull request, you are agreeing to comply
with this waiver of copyright interest.

## Technical overview

Here is how this tool works behind the scenes:

1. User tells the broker that they want a new CDN provisioned (by creating a new instance)
1. The service instance requests a new Let's Encrypt certificate
1. The service instance uploads the challenge file to S3
1. The service instance creates a CloudFront distribution
    * `*` points to Cloud Foundry
    * `/.well-known/acme-challenge/` points to the S3 bucket
1. The user CNAMEs their domain to the CloudFront distribution
1. The service instance waits until it detects the domain is successfully mapped to the proper CloudFront distribution
1. The service instance uploads the certificate to CloudFront

## Development

There are two executables created by this repository: [`cdn-broker`](cmd/cdn-broker/) and [`cdn-cron`](cmd/cdn-cron/). To run them locally:

1. Clone this repository into the `src` directory of your `GOPATH`.
1. Install the dependencies.

    ```bash
    $ go get ./...
    ```

1. Build the binaries.

    ```bash
    $ go build ./cmd/...
    ```

1. Run the binaries via `./cdn-broker` and `./cdn-cron`.
