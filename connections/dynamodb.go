package connections

import (
	"os"
	"sync"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

var (
	dynamoEndpoint = os.Getenv("DYNAMODB_ENDPOINT")
	awsSession     *session.Session
	awsSessionOnce sync.Once
)

// DynamoDB get dynamodb client,
// connects to DYNAMODB_ENDPOINT (e.g. DynamoDB Local) when it is set
func DynamoDB() *dynamodb.DynamoDB {
	awsSessionOnce.Do(func() {
		cfg := aws.NewConfig()
		if dynamoEndpoint != "" {
			cfg = cfg.WithEndpoint(dynamoEndpoint)
		}
		awsSession = session.Must(session.NewSession(cfg))
	})
	return dynamodb.New(awsSession)
}

// dynamoTables are tables and their hash keys used by models
var dynamoTables = map[string]string{
	"boards":   "Board",
	"articles": "Code",
}

// EnsureDynamoDBTables creates missing tables, only works with DYNAMODB_ENDPOINT
func EnsureDynamoDBTables() {
	if dynamoEndpoint == "" {
		return
	}
	db := DynamoDB()
	for table, key := range dynamoTables {
		_, err := db.CreateTable(&dynamodb.CreateTableInput{
			TableName: aws.String(table),
			AttributeDefinitions: []*dynamodb.AttributeDefinition{
				{AttributeName: aws.String(key), AttributeType: aws.String(dynamodb.ScalarAttributeTypeS)},
			},
			KeySchema: []*dynamodb.KeySchemaElement{
				{AttributeName: aws.String(key), KeyType: aws.String(dynamodb.KeyTypeHash)},
			},
			BillingMode: aws.String(dynamodb.BillingModePayPerRequest),
		})
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == dynamodb.ErrCodeResourceInUseException {
			continue
		}
		if err != nil {
			log.WithField("table", table).WithError(err).Fatal("DynamoDB Create Table Failed")
		}
		log.WithField("table", table).Info("DynamoDB Table Created")
	}
}
