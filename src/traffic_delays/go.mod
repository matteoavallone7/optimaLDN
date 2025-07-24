module github.com/matteoavallone7/optimaLDN/src/traffic_delays

go 1.23.3

require (
	github.com/aws/aws-lambda-go v1.49.0
	github.com/influxdata/influxdb-client-go/v2 v2.14.0
	github.com/joho/godotenv v1.5.1
	github.com/rabbitmq/amqp091-go v1.10.0
	github.com/matteoavallone7/optimaLDN/src/common v0.0.0
	github.com/matteoavallone7/optimaLDN/src/rabbitmq v0.0.0
)

replace github.com/matteoavallone7/optimaLDN/src/common => ../common

replace github.com/matteoavallone7/optimaLDN/src/rabbitmq => ../rabbitmq

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/influxdata/line-protocol v0.0.0-20200327222509-2487e7298839 // indirect
	github.com/oapi-codegen/runtime v1.0.0 // indirect
	golang.org/x/net v0.23.0 // indirect
)
