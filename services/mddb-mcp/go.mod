module github.com/tradik/mddb/services/mddb-mcp

go 1.26

require (
	github.com/kelseyhightower/envconfig v1.4.0
	google.golang.org/grpc v1.79.1
	gopkg.in/yaml.v3 v3.0.1
	mddb v0.0.0
)

require (
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace mddb => ../../services/mddbd
