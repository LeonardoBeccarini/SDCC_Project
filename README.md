## Requisiti
  - Docker 
  - Minikube $\geq$ 1.30
  - kubectl $\geq$ 1.27
  - Helm $\geq$ 3.12

## Minikube

1. **deploy del sistema**
	- Rendi eseguibile lo script di deploy : `chmod +x scripts/deploy_minikube.sh`
	- Esegui lo script: `./scripts/deploy_minikube.sh`
2. **Verifica dello stato**: 
	- `kubectl get pods -A`
	- per le pods nei singoli namespace: `kubectl get pods -n <namespace>`
3. **Log di una pod**
	- `kubectl logs -n <namespace> <pod_name>`
4. **Stop & cleanup**
	- Fermare il cluster minikube:   `minikube -p sdcc-cluster stop`
	- Eliminare completamente l'ambiente: `minikube delete --all --purge`

## AWS

### InfluxDB
1. crea un'istanza **EC2** (Linux)
2. da CLI o da UI di InfluxDB (che implica aggiungere una inbound rule tcp con source myIP alla porta 8086 nel security group), creare org e bucket (che andranno poi inseriti nelle env del servizio persistence su EBS)
3. **Security group:** aggiungi una **inbound rule** su 8086 con **source** = **security group dell'istanza EC2** creata da EBS per persistence

### Elasti BeanStalk (EBS)

Creare 3 environment con Docker come piattaforma, abbiamo scelto LabInstanceProfile come IAM Role (l'unico disponibile). Per ogni environment bisogna caricare
un file .zip con il codice sorgente con il Dockerfile nella directory root.

#### Persistence
Nell'environment del servizio persistence nella sezione 'Configurazione' vanno aggiunte le seguenti variabili d'ambiente:
- INFLUX_BUCKET: nome del bucket creato nell'istanza di influxDB su EC2 (aggregated-data)
- INFLUX_ORG: org nell'istanza di influxDB su EC2
- INFLUX_TOKEN: token nfluxDB v2
- INFLUX_URL: `http://<PRIVATE_IP_EC2>:8086`, con l'indirizzo IP privato dell'istanza EC2 con InfluxDB
- MQTT_HOST: `x.tcp.ngrok.io`, indirizzo su cui viene esposto MQTT con ngrok, si trova da minikube con: `kubectl -n rabbitmq logs deploy/ngrok-mqtt | grep -Eo 'tcp://[^ ]+' | tail -n1`
- MQTT_USER, MQTT_PASS: credenziali in `k8s/rabbitmq/config-and-secrets.yaml`
- MQTT_PORT: porta TCP per il tunnel 
- MQTT_TOPIC: `sensor/aggregated/#`
 `
#### Gateway
Nella sezione configurazione vanno aggiunte le seguenti variabili d'ambiente:
- EVENT_URL: resituito da `URL=$(kubectl -n fog logs deploy/event-quick-tunnel | grep -m1 -o 'https://[^ ]*trycloudflare.com')`
- PERSISTENCE_PATH: `/data/latest?source=influx&minutes=1440`
- PERSISTENCE_URL: URL base del servizio `persistence` (DNS EBS dell’env `persistence`)
- PORT: `5009`
#### Dashboard
Per il servizio di dashboard è necessaria come variabile d'ambiente solo GATEWAY_URL, DNS dell'env `gateway`
