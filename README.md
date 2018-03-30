# Code HN

Hacker news with only links from GitHub or GitLab.

__Demo__: [code.hn](https://code.hn)

### Usage

```
$ go run main.go &
$ curl localhost:8080
```

### TODOS

- Add a filter for programming language using github's own API 
- Use channels for results / errors from individual story API requests
- Use some internal scheduler that keeps all the stories in a cache
- Use brute force to ensure we have at least 30 stories for all pages
