package db

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/flashbots/amp-alerts-sink/logutils"
	"go.uber.org/zap"
)

type dynamoDb struct {
	cli       *dynamodb.DynamoDB
	name      string
	namespace string
}

const (
	ddbKeyNamespace = "namespace"
	ddbKeyId        = "id"
	ddbKeyExpireOn  = "expire_on"
	ddbKeyValue     = "value"
)

func newDynamoDb(name string) (*dynamoDb, error) {
	s, err := session.NewSession()
	if err != nil {
		return nil, err
	}

	return &dynamoDb{
		cli:  dynamodb.New(s),
		name: name,
	}, nil
}

func (ddb *dynamoDb) Lock(
	ctx context.Context,
	key string,
	expireIn time.Duration,
) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	input := &dynamodb.PutItemInput{
		TableName: aws.String(ddb.name),

		Item: map[string]*dynamodb.AttributeValue{
			ddbKeyNamespace: {S: aws.String(ddb.namespace)},
			ddbKeyId:        {S: aws.String(key)},

			ddbKeyExpireOn: {N: aws.String(fmt.Sprintf("%d",
				time.Now().Add(expireIn).Unix(),
			))},
		},

		ConditionExpression:      aws.String("attribute_not_exists(#id)"),
		ExpressionAttributeNames: map[string]*string{"#id": aws.String(ddbKeyId)},
	}
	output, err := ddb.cli.PutItemWithContext(ctx, input)

	if err == nil {
		return true, nil
	}
	if _, didCndChkFail := err.(*dynamodb.ConditionalCheckFailedException); didCndChkFail {
		return false, nil
	}

	// error

	logutils.LoggerFromContext(ctx).Error("Dynamo DB failed to lock the key",
		zap.Any("input", input),
		zap.Any("output", output),
		zap.Error(err),
		zap.String("key", key),
	)
	return false, err
}

func (ddb *dynamoDb) Set(
	ctx context.Context,
	key string,
	expireIn time.Duration,
	value string,
) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	ddbItem := map[string]*dynamodb.AttributeValue{
		ddbKeyNamespace: {S: aws.String(ddb.namespace)},
		ddbKeyId:        {S: aws.String(key)},
		ddbKeyValue:     {S: aws.String(value)},

		ddbKeyExpireOn: {N: aws.String(fmt.Sprintf("%d",
			time.Now().Add(expireIn).Unix(),
		))},
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(ddb.name),
		Item:      ddbItem,
	}
	output, err := ddb.cli.PutItemWithContext(ctx, input)

	if err == nil {
		return nil
	}

	// error

	logutils.LoggerFromContext(ctx).Error("Dynamo DB failed to set the key",
		zap.Any("input", input),
		zap.Any("output", output),
		zap.Error(err),
		zap.String("key", key),
	)
	return err
}

func (ddb *dynamoDb) Get(
	ctx context.Context,
	key string,
) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	key = ddb.namespace + "/" + key

	input := &dynamodb.GetItemInput{
		TableName: aws.String(ddb.name),

		Key: map[string]*dynamodb.AttributeValue{
			ddbKeyId: {S: aws.String(key)},
		},
	}
	output, err := ddb.cli.GetItemWithContext(ctx, input)
	if err != nil {
		logutils.LoggerFromContext(ctx).Error("Dynamo DB failed to get the key",
			zap.Any("input", input),
			zap.Any("output", output),
			zap.Error(err),
			zap.String("key", key),
		)
		return "", err
	}

	if len(output.Item) == 0 {
		return "", nil
	}

	value := output.Item[ddbKeyValue]
	switch {
	// TODO: fill-in other case?
	case value.S != nil:
		return *value.S, nil
	default:
		return "", nil
	}
}

func (ddb *dynamoDb) WithNamespace(namespace string) DB {
	return &dynamoDb{
		cli:       ddb.cli,
		name:      ddb.name,
		namespace: namespace,
	}
}
