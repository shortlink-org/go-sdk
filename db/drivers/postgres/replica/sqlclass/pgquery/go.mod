module github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/sqlclass/pgquery

go 1.27.0

require (
	github.com/pganalyze/pg_query_go/v6 v6.2.2
	github.com/shortlink-org/go-sdk/db v0.0.0-20260914110947-2237285eb582
	github.com/stretchr/testify v1.12.1
	google.golang.org/protobuf v1.36.12
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

replace github.com/shortlink-org/go-sdk/db => ../../../../..
