FROM grafana/k6:latest

COPY xk6-gcp /usr/bin/xk6-gcp

RUN k6 plugin install xk6-gcp

ENTRYPOINT ["k6"]