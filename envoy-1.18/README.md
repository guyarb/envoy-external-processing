### Envoy External Processing Filter

A really basic implementation of envoy [External Processing Filter](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/ext_proc/v3alpha/ext_proc.proto#external-processing-filter).  This capability allows you to define an external gRPC server which can selectively process headers and payload/body of requests (see [External Processing Filter PRD](https://docs.google.com/document/d/1IZqm5IUnG9gc2VqwGaN5C2TZAD9_QbsY9Vvy5vr9Zmw/edit#heading=h.3zlthggr9vvv).  Basically, your own unrestricted filter.

```
          ext_proc 
             ^
             |
client ->  envoy -> upstream
```

>> NOTE, this filter is really early and has a lot of features to implement!

- Source: [ext_proc.cc](https://github.com/envoyproxy/envoy/blob/main/source/extensions/filters/http/ext_proc/ext_proc.cc)

---

All we will demonstrate in this repo is the most basic functionality: manipulate headers and body-content on the request/response.  I know, there are countless other ways to do this with envoy but just as a demonstration of writing the external gRPC server that this functionality uses. If interested, pls read on:

The scenario is like this


A) Manipulate outbound headers and body

```
          ext_proc   (delete specific header from client to upstream; append body content sent to upstream)
             ^
             |
client ->  envoy -> upstream
```

B) Manipulate response headers and body

```
          ext_proc   (delete specific header from upstream to client; append body content sent to client)
             ^
             |
client <-  envoy <- upstream
```

```bash
docker cp `docker create  envoyproxy/envoy-dev:latest`:/usr/local/bin/envoy .
```

Now start the external gRPC server

```bash
go run grpc_server.go
```

Now start envoy

```
./envoy -c server.yaml -l debug
```

Note, the external processing filter is by default configured to ONLY ask for the inbound request headers.  What we're going to do in code is first check if the header contains the specific value we're interested in (i.,e header has a 'user' in it), if so, then we will ask for the request body, which will ask for the response headers which inturn will override and ask for the response body

```yaml
          http_filters:
          - name: envoy.filters.http.ext_proc
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3alpha.ExternalProcessor
              failure_mode_allow: false
              async_mode: false              
              request_attributes:
              - user
              response_attributes:
              - server
              processing_mode:
                request_header_mode: "SEND"
                response_header_mode: "SEND"
                request_body_mode: "BUFFERED"
                response_body_mode: "BUFFERED"
                request_trailer_mode: "SKIP"
                response_trailer_mode: "SKIP"
              grpc_service:
                envoy_grpc:                  
                  cluster_name: ext_proc_cluster
```

```bash
$ curl -v -H "host: http.domain.com"  --resolve  http.domain.com:8080:127.0.0.1  http://http.domain.com:8080/get
```