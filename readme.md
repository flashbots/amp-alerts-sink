# amp-alerts-sink

Receives alerts from AMP via SNS and dispatches them to configured destinations.

## TL;DR

```shell
amp-alerts-sink lambda \
  --processor-ignore-rules DatasourceError \
  --processor-match-labels foo=bar \
  --publisher-slack-channel-id XXXXXXXXXXX \
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

## Webhook publisher

The webhook publisher sends alerts to an arbitrary HTTP endpoint. It supports deduplication via DynamoDB to prevent duplicate deliveries across concurrent Lambda invocations.

### Configuration

| Flag                            | Env var                                       | Description                                      |
| ------------------------------- | --------------------------------------------- | ------------------------------------------------ |
| `--publisher-webhook-url`       | `AMP_ALERTS_SINK_PUBLISHER_WEBHOOK_URL`       | Webhook URL (raw URL or AWS Secrets Manager ARN) |
| `--publisher-webhook-method`    | `AMP_ALERTS_SINK_PUBLISHER_WEBHOOK_METHOD`    | HTTP method (default: `POST`)                    |
| `--publisher-webhook-send-body` | `AMP_ALERTS_SINK_PUBLISHER_WEBHOOK_SEND_BODY` | Send alert as JSON body (default: `true`)        |

### Request format

When `send-body` is enabled, the webhook sends a JSON payload compatible with the Alertmanager webhook receiver format (`notify/webhook.Message`):

```json
{
  "version": "4",
  "groupKey": "<dedup key>",
  "receiver": "<source>",
  "status": "<status>",
  "alerts": [{ "...": "..." }]
}
```

When `send-body` is disabled, a request with no body is sent to the configured URL (useful for simple trigger-style webhooks).
