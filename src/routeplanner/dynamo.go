package routeplanner

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/matteoavallone7/optimaLDN/src/common"
	"log"
)

func GetActiveRoute(ctx context.Context, userID string) (*common.ChosenRoute, error) {
	input := &dynamodb.GetItemInput{
		TableName: aws.String("ChosenRoutes"),
		Key: map[string]types.AttributeValue{
			"UserID": &types.AttributeValueMemberS{Value: userID},
		},
	}

	result, err := dbClient.GetItem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get active route: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("no active route found for user %s", userID)
	}

	var route common.ChosenRoute
	err = attributevalue.UnmarshalMap(result.Item, &route)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal route: %w", err)
	}

	return &route, nil
}

func SaveChosenRoute(ctx context.Context, route common.ChosenRoute) error {
	item, err := attributevalue.MarshalMap(route)
	if err != nil {
		return fmt.Errorf("failed to marshal chosen route: %w", err)
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String("ChosenRoutes"),
		Item:      item,
	}

	_, err = dbClient.PutItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to save route to DynamoDB: %w", err)
	}

	log.Printf("Saved chosen route for user %s with %d legs.", route.UserID, len(route.Legs))
	return nil
}

func DeleteChosenRoute(ctx context.Context, userID string) error {
	key, err := attributevalue.MarshalMap(map[string]string{
		"UserID": userID,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal key: %w", err)
	}

	input := &dynamodb.DeleteItemInput{
		TableName: aws.String("ChosenRoutes"),
		Key:       key,
	}

	_, err = dbClient.DeleteItem(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to delete route for user %s: %w", userID, err)
	}

	log.Printf("Deleted chosen route for user %s", userID)
	return nil
}
