# Using golang alpine image to build the binary
FROM golang:1.25-alpine3.21 as build

# Build arguments. specified via buil-arg
ARG SERVICE="nubitel-dialplan-stats"
ARG VERSION=${VERSION}


# Set GOPATH environment variable
ENV GOPATH /Nubitel/go

# Set working dir 
WORKDIR ${GOPATH}/src/${SERVICE}

# Copy the whole code
COPY . .

# Build it
RUN go build -ldflags "-w -s -X main.Version=${VERSION} -X main.BuildTime=`date +%FT%T%z`" -mod vendor -o /${SERVICE}

# Use alpine for deployment as it got minimal footprint
FROM alpine:3.21 as deploy

ARG SERVICE="nubitel-dialplan-stats"

# Copy the service image from previous build into this 
COPY --from=build /${SERVICE} .

RUN apk update && apk add bash && apk --no-cache add tzdata

COPY .env .
# Port exposed
EXPOSE 5012

# Start service
CMD ["/dialplan-manager-stats", "-host=0.0.0.0", "-port=5012"]