# ghproxy

这是 [gh-proxy](https://github.com/hunshcn/gh-proxy) 的 Go 语言版本。

## 支持内容

• GitHub API (api.github.com)

• Raw 文件 (raw.githubusercontent.com)

• Gist 文件 (gist.githubusercontent.com)

• Releases 文件 (github.com/用户名/仓库名/releases|archive/)

• Git 克隆支持 (git clone)


## Docker 部署

1. 新建配置文件 `config.json`，内容如下：
```json
{
  "domain": "gh.llkk.cc",
  "whiteList": [
  ],
  "blackList": [
    "example1",
    "example2"
  ]
}
```
> 配置文件中的 `domain` 是用于替换前端页面使用说明中的域名，`whiteList` 和 `blackList` 分别是白名单和黑名单，如果设置白名单。则只有白名单中的域名会被代理，黑名单可用于过滤部分不安全仓库。

2. docker-compose.yml
```yaml
services:
  ghproxy:
    # image: oopsunix/ghproxy:latest # Docker Hub 镜像源
    # image：ghcr.io/oopsunix/ghproxy:latest # GitHub 镜像源
    image: registry.cn-hangzhou.aliyuncs.com/oopsunix/ghproxy:latest # 国内环境使用阿里云镜像源
    container_name: ghproxy
    restart: unless-stopped
    volumes:
      - ./config.json:/app/config.json # 配置文件挂载路径，根据实际情况修改
    ports:
      - "45001:45000" # 端口映射，根据实际情况修改
```

3. 启动容器
```bash
docker-compose up -d
```


---

如有问题或改进建议，欢迎提交 [issue] 或 PR！


[issue]: https://github.com/oopsunix/ghproxy/issues