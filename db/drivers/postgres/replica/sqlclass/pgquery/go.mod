module github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass/pgquery

go 1.26.2

require (
	github.com/pganalyze/pg_query_go/v6 v6.2.2
	github.com/shortlink-org/go-sdk/db v0.0.0-00010101000000-000000000000
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/stretchr/testify v1.12.1
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

replace github.com/shortlink-org/go-sdk/db => ../../../../..
