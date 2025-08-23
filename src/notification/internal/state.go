package internal

import (
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/patrickmn/go-cache"
)

var (
	DBClient *dynamodb.Client
	AppCache *cache.Cache
)
