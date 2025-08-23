package internal

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"github.com/patrickmn/go-cache"
	"log"
)

func CheckActiveRoutes(ctx context.Context, lineName string) (bool, []string) {
	cacheKey := fmt.Sprintf("%s", lineName)

	// 1. Try to get from cache
	if cachedUsers, found := AppCache.Get(cacheKey); found {
		if userIDs, ok := cachedUsers.([]string); ok && len(userIDs) > 0 {
			log.Printf("Cache HIT for %s (Line: %s)", cacheKey, lineName)
			return true, userIDs
		}
		// If type assertion fails, it means something unexpected is in cache,
		// so we'll treat it as a cache miss and re-fetch.
		log.Printf("Cache entry for %s found but invalid type. Re-fetching.", cacheKey)
	}
	log.Printf("Cache MISS for %s (Line: %s). Querying DynamoDB...", cacheKey, lineName)

	input := &dynamodb.ScanInput{
		TableName:        aws.String("ActiveRoutes"),
		FilterExpression: aws.String("LineName = :line"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":line": &types.AttributeValueMemberS{Value: lineName},
		},
	}

	result, err := DBClient.Scan(ctx, input)
	if err != nil {
		log.Printf("Failed to scan DynamoDB: %v", err)
		return false, nil
	}

	var routes []common.ActiveRoute
	err = attributevalue.UnmarshalListOfMaps(result.Items, &routes)
	if err != nil {
		log.Printf("Failed to unmarshal items: %v", err)
		return false, nil
	}

	var foundUserIDs []string
	for _, route := range routes {
		foundUserIDs = append(foundUserIDs, route.UserID)
	}

	if len(foundUserIDs) > 0 {
		AppCache.Set(cacheKey, foundUserIDs, cache.DefaultExpiration)
		log.Printf("Cached %d user(s) for line %s.", len(foundUserIDs), lineName)
	} else {
		log.Printf("No users found for line %s. Nothing cached.", lineName)
	}

	return len(foundUserIDs) > 0, foundUserIDs
}

func RegisterNewRoute(ctx context.Context, route common.ActiveRoute) error {
	item, err := attributevalue.MarshalMap(route)
	if err != nil {
		return fmt.Errorf("failed to marshal ActiveRoute: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String("ActiveRoutes"),
		Item:      item,
	}

	_, err = DBClient.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to write ActiveRoute to DynamoDB: %w", err)
	}
	return nil
}

func DeleteActiveRoute(ctx context.Context, route common.ActiveRoute) (*common.ActiveRoute, error) {
	key, err := attributevalue.MarshalMap(map[string]string{
		"UserID": route.UserID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key: %w", err)
	}

	input := &dynamodb.DeleteItemInput{
		TableName:    aws.String("ActiveRoutes"),
		Key:          key,
		ReturnValues: types.ReturnValueAllOld, // Ensure deleted item is returned
	}

	result, err := DBClient.DeleteItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to delete active route for user %s: %w", route.UserID, err)
	}

	if len(result.Attributes) == 0 {
		log.Printf("Info: No active route found for user %s to delete.", route.UserID)
		return nil, nil
	}

	var deletedRoute common.ActiveRoute
	err = attributevalue.UnmarshalMap(result.Attributes, &deletedRoute)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal deleted item: %w", err)
	}

	log.Printf("Successfully deleted active route for user %s on line %s.", deletedRoute.UserID, deletedRoute.LineIDs)
	return &deletedRoute, nil
}
