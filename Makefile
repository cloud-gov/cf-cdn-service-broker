.PHONY: test
test:
	trap "docker stop postgres-11; docker rm -f postgres-11" EXIT
	docker run -p 5432:5432 --name postgres-11 -e POSTGRES_PASSWORD=foobar -d postgres:11.5

	POSTGRES_URL="postgres://postgres:foobar@localhost/postgres?sslmode=disable" \
		go run github.com/onsi/ginkgo/v2/ginkgo -v -r

.PHONY: fakes
fakes:
	go generate ./...

.PHONY: build_amd64
build_amd64:
	mkdir -p amd64
	GOOS=linux GOARCH=amd64 go build -o amd64/cdn-broker github.com/alphagov/paas-cdn-broker/cmd/cdn-broker
	GOOS=linux GOARCH=amd64 go build -o amd64/cdn-cron github.com/alphagov/paas-cdn-broker/cmd/cdn-cron

.PHONY: bosh_scp
bosh_scp: build_amd64
	./scripts/bosh-scp.sh
