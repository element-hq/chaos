### mitmproxy

This directory contains code that will be used as a [mitmproxy addon](https://docs.mitmproxy.org/stable/addons-overview/).

How this works:
 - A vanilla `mitmproxy` is run in the same network as the homeservers, to which a Flask HTTP server is attached.
 - Homeservers are configured to talk to mitmproxy via `HTTP_PROXY` env vars.
 - The Flask HTTP server can be used to control mitmproxy at test runtime.

### Callback addon

A [mitmproxy addon](https://docs.mitmproxy.org/stable/addons-examples/) bolts on custom
functionality to mitmproxy. This typically involves using the
[Event Hooks API](https://docs.mitmproxy.org/stable/api/events.html) to listen for
[HTTP flows](https://docs.mitmproxy.org/stable/api/mitmproxy/http.html#HTTPFlow).

The `callback` addon is a Complement-Crypto specific addon which calls a client provided URL
mid-flow, with a JSON object containing information about the HTTP flow. The caller can then
return another JSON object which can modify the response in some way.

Available configuration options are optional:
 - `callback_request_url`: the URL to send outbound requests to. This allows callbacks to intercept
   requests BEFORE they reach the server.
 - `callback_response_url`: the URL to send inbound responses to. This allows callbacks to modify
   response content.
 - `filter`: the [mitmproxy filter](https://docs.mitmproxy.org/stable/concepts-filters/) to apply. If unset, ALL requests are eligible to go to the callback
   server.

To use this with the controller API, you would send an HTTP request like this:
```js
{
  "options": {
    "callback": {
      "callback_response_url": "http://host.docker.internal:445566/response"
    }
  }
}
```

#### `callback_request_url`

mitmproxy will POST to `callback_request_url` with the following JSON object:
```js
{
   method: "GET|PUT|...",
   access_token: "syt_11...",
   url: "http://hs1/_matrix/client/...",
   request_body: { some json object or null if no body },
}
```
The callback server can then either return an empty object or the following object (all fields are required):
```js
{
   respond_status_code: 200,
   respond_body: { "some": "json_object" }
}
```
If an empty object is returned, mitmproxy will forward the request unaltered to the server. If the above object (with all fields set) is returned, mitmproxy will send that response _immediately_ and **will not send the request to the server**. This can be used to block HTTP requests.


#### `callback_response_url`
Similarly, mitmproxy will POST to `callback_response_url` with the following JSON object:
```js
{
   method: "GET|PUT|...",
   access_token: "syt_11...",
   url: "http://hs1/_matrix/client/...",
   request_body: { some json object or null if no body },
   // note these are new fields because the request was sent to the HS and a response returned from it
   response_body: { some json object },
   response_code: 200,
}
```
The callback server can then return optional keys to replace parts of the response.
The values returned here will be returned to the Matrix client:
```js
{
   respond_status_code: 200,
   respond_body: { "some": "json_object" }
}
```
These keys are optional. If neither are specified, the response is sent unaltered to
the Matrix client. If the body is set but the status code is not, only the body is
modified and the status code is left unaltered and vice versa.