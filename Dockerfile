ARG CI_PROJECT_NAME

FROM harbor.dell.com/devops-images/debian-12/go-1.23:latest

COPY --from=harbor.dell.com/observability-monitoring/dynatrace/oneagent-codemodules:latest / /
ENV LD_PRELOAD=/opt/dynatrace/oneagent/agent/lib64/liboneagentproc.so

WORKDIR /app

COPY target/${CI_PROJECT_NAME} /app/main

RUN chmod +x /app/main

EXPOSE 8000

CMD ["./main/b2b-inbound-router-service"] 
