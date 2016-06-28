# gsync

sync files between client and server

##features

- sync app directory like rsync
- client and server communication based on http
- client will not delete but noly update files
- server can serve many apps(different directories) the same time

##usage example

- config server in config.txt and startup server: `server`
- run client in app folder: `client -v -h "localhost:8088"`

##notes
sample client config(file named .autoconfig and  under the same directory where client belongs):
```
{
	"SyncHost":"127.0.0.1:8088",
	"SyncApp":"client",
	"Ignore":[".autoupdate","client.exe", "logs/*.log"],
	"LastUpdate":"2016-06-28T10:29:34.8880669+08:00"
}
```


sample server config:
```
{
    "listen": ":8088",
    "cachedir": "cache",
    "apps" : {
        "client" : {
            "dir": "publish/client"
        },
        "app2" : {
            "dir" : "publish/app2"
        }
    }
}
```
