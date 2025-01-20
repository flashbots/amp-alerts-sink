# amp-alerts-sink

Receives alerts from AMP via SNS and dispatches them to configured destinations.

## TL;DR

```shell
amp-alerts-sink lambda \
  --processor-ignore-rules DatasourceError \
  --processor-match-labels foo=bar \
  --publisher-slack-channel-id XXXXXXXXXXX \
  --publisher-slack-channel-name alerts \
  --publisher-slack-token arn:aws:secretsmanager:rrr:aaa:secret:sss
```

## DynamoDB

`amp-alerts-sink` uses dynamo db for alerts deduplication and tracking.
Required schema can be deployed with the following terraform code:

```terraform
resource "aws_dynamodb_table" "amp_alerts_sink" {
  name         = "amp-alerts-sink"
  billing_mode = "PAY_PER_REQUEST"

  hash_key  = "namespace"
  range_key = "id"

  attribute {
    name = "namespace"
    type = "S"
  }

  attribute {
    name = "id"
    type = "S"
  }

  ttl {
    attribute_name = "expire_on"
    enabled        = true
  }
}
```
