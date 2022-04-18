# reverse-hash-service

Store and serve poseidon hashes.

## Run service



```console
# create database
createdb rhs && psql -d rhs < ./schema.sql

# default database URL is postgers://rhs@localhost with local auth
export RHS_DB="host=localhost password=pgpwd user=postgres database=rhs"

# default listen address is :8080
# export RHS_LISTEN_ADDR=:8080

go build && ./reverse-hash-service
```

## Save new hashes

```console
curl -H "Content-Type: application/json" -X POST localhost:8080/node -d '[
  {
    "hash": "e33d2335edfc794a855cbfd235a7e9e8ea433e569591012cd743c17fa6a02b1e",
    "children": [
      "5fb90badb37c5821b6d95526a41a9504680b4e7c8b763a1b1d49d4955c848621",
      "65f606f6a63b7f3dfd2567c18979e4d60f26686d9bf2fb26c901ff354cde1607"
    ]
  },
  {
    "hash": "c5df774d59b69814c679868deaf42354dc5de89e34088c4a1dbbf362d703b314",
    "children": [
      "5d27606e29afb1fde4f6764fa0a01eec23e11dafffabae96ed2ae7229aa5992a",
      "bc4dd02832954c16a6ce4c48da20fe517e822caa6dc3fabfcdf9684443321002"
    ]
  }
]'
# Output:
# {"status":"OK"}
```

## Retrieve hash

```console
curl localhost:8080/node/e33d2335edfc794a855cbfd235a7e9e8ea433e569591012cd743c17fa6a02b1e
# Output:
# {
#   "status": "OK",
#   "node": {
#     "hash": "e33d2335edfc794a855cbfd235a7e9e8ea433e569591012cd743c17fa6a02b1e",
#     "children": [
#       "5fb90badb37c5821b6d95526a41a9504680b4e7c8b763a1b1d49d4955c848621",
#       "65f606f6a63b7f3dfd2567c18979e4d60f26686d9bf2fb26c901ff354cde1607"
#     ]
#   }
# }
