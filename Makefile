.PHONY: test
test:
	trap "docker stop postgres-11; docker rm -f postgres-11" EXIT
	docker run -p 5432:5432 --name postgres-11 -e POSTGRES_PASSWORD=foobar -d postgres:11.5

	POSTGRES_URL="postgres://postgres:foobar@localhost/postgres?sslmode=disable" \
		ginkgo -v -r
