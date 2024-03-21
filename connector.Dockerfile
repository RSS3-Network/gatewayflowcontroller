FROM golang:alpine AS Builder

WORKDIR /app

#RUN apk add \
#    gcc \
#    g++

COPY . .

RUN go mod download

RUN go build -o app ./connector/main

FROM scratch AS Runner

WORKDIR /app

COPY --from=Builder /app/app /app/app

EXPOSE 1234/tcp

CMD ["/app/app"]
