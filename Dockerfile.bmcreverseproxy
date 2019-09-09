FROM nginx:1.17
LABEL maintainer FI-TS Devops <devops@f-i-ts.de>

COPY nginx.conf /etc/nginx/
COPY server.crt /etc/ssl/certs/
COPY server.key /etc/ssl/private/
COPY client.crt /etc/nginx/
