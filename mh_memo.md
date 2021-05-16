# OAUTH2 + DID BAS

<br><br><br><br>

## - Contribution

 - Front-Side & Performace Check Server

<br><br><br><br>

## - 주요 Directory
 - Client
 - Server
 - bas_token_basic

<br><br><br><br>

## - 실행 순서

1. test-network/ 로 이동
```cmd
./network.sh down
```
```cmd
./network.sh up createChannel -ca -s couchdb
```
```cmd
./network.sh deployCC -ccn basic -ccp ../bas_token_basic/chaincode-go/ -ccl go
```

2. server/ 로 이동

```cmd
go run server.go
```

3. client/ 로 이동
```cmd
go run client.go 10 10
```

##### (만약 oauth2가 설치되어 있지 않다면)
```cmd
go get golang.org/x/oauth2
```

<br><br><br><br>

## - 주요 메서드 (bcconnector.go)

### * newConnector()
### * GetCodeInfoByDID()
```js
[
   {
      "InfoType":"CodeInfo",
      "ID_code":"TestCID",
      "DID_RO":"TestDIDRO",
      "DID_client":"TESTDIDClient",
      "Scope":"all",
      "Hash_code":"TestCHV",
      "Time_issueed":"TestTime",
      "URI_Redirection":"TESTURI",
      "Condition":"Available",
      "ID_token":"TestTID"
   }
]
```

### * GetTokenInfoByDID()
```js
[
   {
      "InfoType":"TokenInfo",
      "ID_token":"TestTID",
      "DID_RO":"TestDIDRO",
      "DID_client":"TESTDIDClient",
      "Scope":"all",
      "Hash_code":"TestTHV",
      "Time_issueed":"TestTime",
      "Time_expiration":"TestDuration",
      "URI_Redirection":"TESTURI",
      "Condition":"Available"
   }
]
```

<br><br><br><br>

## - 참고사항

9096 : 서버

9094 : 클라이언트

### * 데이터 형식



