## Initial Architecture


![cluster (9)](https://github.com/ph-ngn/nanobox/assets/93941060/6634b5a4-4f7a-4f45-87a5-4513cce8ad63)




## Tech roadmap:
- Raft consensus library: https://github.com/hashicorp/raft?tab=readme-ov-file
- OpenTelemetry: https://opentelemetry.io/
- Zap: https://github.com/uber-go/zap
- Grafana (Loki + Tempo): https://grafana.com/
- InfluxDB: https://www.influxdata.com/
- Docker, K8s, Helm, AWS

## Design:
### nbox-api-server:
-  Summary: Entry point into the box cluster
### box:
-  Summary: In-memory key-value store
### fsm: 
-  Summary: State machine replication
### kube:
-  Summary: Kubernetes middleware
