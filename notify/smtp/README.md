
## Endly inline workflow

```bash
endly -r=send
```

[send.yaml](test/send.yaml)

```yaml
init:
defaults:
  target:
    URL: smtp://smtp.gmail.com:465
    credentials: smtp
  sender: viantemailtester@gmail.com

pipeline:
  send:
    action: smtp:send
    udf: Markdown
    mail:
      to:
      - awitas@viantinc.com
      from: $sender
      subject: Endly test
      contentType: text/html
      body:  "# test message\n
              * list item 1\n
              * list item 2"

```




<a name="endly"></a>
## Endly service action integration

Run the following command for exec service operation details:

```bash
endly -s=smtp -a=send
```


**SMTP Service**

| Service Id | Action | Description | Request | Response |
| --- | --- | --- | --- | --- | 
| smtp | send | send an email to supplied recipients | [SendRequest](service_smtp_send.go#L10) | [SendResponse](service_smtp_send.go#L17) | 



**RunRequest example**


@run.json

```json
{
  "Target": {
    "URL": "ssh://127.0.0.1/",
    "Credentials": "${env.HOME}/.secret/localhost.json"
  },
  "SuperUser":true,
  "Commands":["mkdir /tmp/app1"]
}
```



```run.yaml

init:
defaults:
  target:
    URL: smtp://smtp.gmail.com:465
    credentials: smtp
  sender: viantemailtester@gmail.com

pipeline:
  send:
    action: smtp:send
    udf: Markdown
    mail:
      to:
      - awitas@viantinc.com
      from: $sender
      subject: Endly test
      contentType: text/html
      body:  "# test message\n
              * list item 1\n
              * list item 2"

```


**Catching and seding errors**


[@run.yaml](test/send_err.yaml)
```yaml
init:
  - "body = Error: <strong>$error.Error at:</strong>
    $error.TaskName:
    <br />
    <code>
    $errorJSON
    </code>"
defaults:
  target:
    URL: smtp://smtp.gmail.com:465
    credentials: smtp
  sender: viantemailtester@gmail.com

pipeline:

  task1:
    action: fail
    message: test failure

  catch:
    action: smtp:send
    mail:
      to:
      - awitas@viantinc.com
      from: $sender
      subject: Endly test $error.Error
      contentType: text/html
      body: $body
  defer:
    action: print
    message: all done
```