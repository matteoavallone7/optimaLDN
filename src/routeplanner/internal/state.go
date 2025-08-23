package internal

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/matteoavallone7/optimaLDN/src/rabbitmq"
)

var (
	RoutePublisher *rabbitmq.Publisher
	DBClient       *dynamodb.Client
	TflAPIKey      string
	NaptanMap      = make(map[string]string)
)
