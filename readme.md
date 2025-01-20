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
