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

## A2A publisher

The A2A publisher discovers an agent through its standard Agent Card and sends
the alert with the A2A `SendMessage` operation.

### A2A Configuration

| Flag                              | Env var                                         | Description                                                  |
| --------------------------------- | ----------------------------------------------- | ------------------------------------------------------------ |
| `--publisher-a2a-bearer-token`    | `AMP_ALERTS_SINK_PUBLISHER_A2A_BEARER_TOKEN`    | Optional bearer token (raw value or AWS Secrets Manager ARN) |
| `--publisher-a2a-prompt-template` | `AMP_ALERTS_SINK_PUBLISHER_A2A_PROMPT_TEMPLATE` | Go template used to render the agent prompt                  |
| `--publisher-a2a-prompt-url`      | `AMP_ALERTS_SINK_PUBLISHER_A2A_PROMPT_URL`      | Base A2A URL of the agent                                    |
| `--publisher-a2a-timeout`         | `AMP_ALERTS_SINK_PUBLISHER_A2A_TIMEOUT`         | A2A messages timeout (default: `30s`)                        |

The prompt template receives these values:

- `.Source`: SNS topic ARN or the synthetic alert source.
- `.Alert`: the complete `AlertmanagerAlert`.
- `.Publishers`: results from publishers that ran earlier. Each result has
  `.Status`, `.Metadata`, and, on failure, `.Error`.
- `toJSON`: a helper for rendering any value as compact JSON.

For example:

```gotemplate
Investigate {{ .Alert.Labels.alertname }} ({{ .Alert.Status }}).
Slack channel: {{ .Publishers.slack.Metadata.channel_id }}
Slack thread: {{ .Publishers.slack.Metadata.thread_ts }}
Alert: {{ toJSON .Alert }}
```

In addition to the rendered text part, the A2A message contains an
`application/json` data part with `source`, `alert`, and `publishers`. A2A task
or message identifiers returned by the agent are saved with the deduplication
record and exposed as this publisher's metadata.

## Webhook publisher

The webhook publisher sends alerts to an arbitrary HTTP endpoint. It supports
deduplication via DynamoDB to prevent duplicate deliveries across concurrent
Lambda invocations.

### Webhook Configuration

| Flag                            | Env var                                       | Description                                      |
| ------------------------------- | --------------------------------------------- | ------------------------------------------------ |
| `--publisher-webhook-url`       | `AMP_ALERTS_SINK_PUBLISHER_WEBHOOK_URL`       | Webhook URL (raw URL or AWS Secrets Manager ARN) |
| `--publisher-webhook-method`    | `AMP_ALERTS_SINK_PUBLISHER_WEBHOOK_METHOD`    | HTTP method (default: `POST`)                    |
| `--publisher-webhook-send-body` | `AMP_ALERTS_SINK_PUBLISHER_WEBHOOK_SEND_BODY` | Send alert as JSON body (default: `true`)        |

### Request format

When `send-body` is enabled, the webhook sends a JSON payload compatible with
the Alertmanager webhook receiver format (`notify/webhook.Message`):

```json
{
  "version": "4",
  "groupKey": "<dedup key>",
  "receiver": "<source>",
  "status": "<status>",
  "alerts": [{ "...": "..." }]
}
```

When `send-body` is disabled, a request with no body is sent to the configured
URL (useful for simple trigger-style webhooks).
