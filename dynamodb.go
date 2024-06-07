package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

var (
	db *dynamodb.Client
)

func listTable() {
	// Build the request with its input parameters
	resp, err := db.ListTables(context.TODO(), &dynamodb.ListTablesInput{
		Limit: aws.Int32(5),
	})
	if err != nil {
		log.Fatalf("failed to list tables, %v", err)
	}

	for _, tableName := range resp.TableNames {
		fmt.Println(tableName)
	}
}

func setupDBClient() error {
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
				SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			}, nil
		})),
	)
	if err != nil {
		return fmt.Errorf("unable to load SDK config, %v", err)
	}
	db = dynamodb.NewFromConfig(cfg)
	return nil
}

func ensureTableExists(tableName string) error {
	_, err := db.DescribeTable(context.TODO(), &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err == nil {
		fmt.Printf("Table %s already exists\n", tableName)
		return nil
	}

	var resourceNotFoundException *types.ResourceNotFoundException
	if !errors.As(err, &resourceNotFoundException) {
		return fmt.Errorf("failed to describe table: %v", err)
	}

	fmt.Printf("Creating table %s\n", tableName)
	_, err = db.CreateTable(context.TODO(), &dynamodb.CreateTableInput{
		TableName: aws.String(tableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("ID"),
				KeyType:       types.KeyTypeHash, // Partition key
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("ID"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		ProvisionedThroughput: &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(5),
			WriteCapacityUnits: aws.Int64(5),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create table, %v", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(db)
	err = waiter.Wait(context.TODO(), &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to wait for table to become active, %v", err)
	}

	fmt.Printf("Table %s created successfully\n", tableName)
	return nil
}

func getTableContents(tableName string) (*dynamodb.ScanOutput, error) {
	ensureTableExists(tableName)
	result, err := db.Scan(context.TODO(), &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to scan table, %v", err)
	}
	return result, nil
}

func getAllFields(tableName string) ([]string, error) {
	// Describe the table to get its schema
	resp, err := db.DescribeTable(context.TODO(), &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe table, %v", err)
	}

	// Extract the attribute names
	var fields []string
	for _, attr := range resp.Table.AttributeDefinitions {
		fields = append(fields, aws.ToString(attr.AttributeName))
	}
	return fields, nil
}

func UpdateOrInsertItem(tableName string, item Item) error {
	// Check if the item already exists
	_, err := db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"ID": &types.AttributeValueMemberS{Value: item.ID},
		},
	})
	if err != nil {
		if reqerr, ok := err.(awserr.RequestFailure); ok && reqerr.StatusCode() == 404 {
			// Item not found, insert it
			putItemInput := &dynamodb.PutItemInput{
				TableName: aws.String(tableName),
				Item: map[string]types.AttributeValue{
					"ID":          &types.AttributeValueMemberS{Value: item.ID},
					"LastUpdated": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
					"Injured":     &types.AttributeValueMemberN{Value: strconv.Itoa(item.Injured)},
					"Deaths":      &types.AttributeValueMemberN{Value: strconv.Itoa(item.Deaths)},
					"Magnitude":   &types.AttributeValueMemberN{Value: strconv.FormatFloat(item.Magnitude, 'f', -1, 32)},
					"#Loc":        &types.AttributeValueMemberS{Value: item.Location}, // Use a placeholder for 'Location'
					"#Date":       &types.AttributeValueMemberS{Value: item.Date},
					"RefUrl":      &types.AttributeValueMemberS{Value: item.RefUrl},
				},
				ConditionExpression: aws.String("attribute_not_exists(ID)"),
				ExpressionAttributeNames: map[string]string{
					"#Loc":  "Location",
					"#Date": "Date",
				},
			}
			_, err := db.PutItem(ctx, putItemInput)
			if err != nil {
				return fmt.Errorf("failed to insert item: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to query item: %w", err)
	}

	// Item found, update it
	updateItemInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]types.AttributeValue{
			"ID": &types.AttributeValueMemberS{Value: item.ID},
		},
		UpdateExpression: aws.String("SET LastUpdated = :lastUpdated, Injured = :injured, Deaths = :deaths, Magnitude = :magnitude, #Loc = :location, #Date = :date, RefUrl = :refUrl"), // Use a placeholder for 'Location'
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":lastUpdated": &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339Nano)},
			":injured":     &types.AttributeValueMemberN{Value: strconv.Itoa(item.Injured)},
			":deaths":      &types.AttributeValueMemberN{Value: strconv.Itoa(item.Deaths)},
			":magnitude":   &types.AttributeValueMemberN{Value: strconv.FormatFloat(item.Magnitude, 'f', -1, 32)},
			":location":    &types.AttributeValueMemberS{Value: item.Location},
			":date":        &types.AttributeValueMemberS{Value: item.Date},
			":refUrl":      &types.AttributeValueMemberS{Value: item.RefUrl},
		},
		ExpressionAttributeNames: map[string]string{
			"#Loc":  "Location",
			"#Date": "Date",
		},
	}
	_, err = db.UpdateItem(ctx, updateItemInput)
	if err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}
	return nil
}
