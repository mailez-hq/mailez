# mailez engine-lb：多活引擎的邮件端口 L4 负载均衡（haproxy TCP 透传）。
# 复制经 Docker DNS + server-template 动态展开；配置见 haproxy.cfg 头部说明。
FROM haproxy:2.9-alpine

COPY haproxy.cfg /usr/local/etc/haproxy/haproxy.cfg

EXPOSE 25 587 1587 465 143 993 110 995 4190
