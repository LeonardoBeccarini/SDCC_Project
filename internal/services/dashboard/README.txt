Elastic Beanstalk (Docker) bundle

Files:
- app.py (Flask + requests)
- templates/dashboard.html
- static/style.css
- requirements.txt
- Dockerfile

After upload:
- In the EB console, set a Software configuration env var:
  GATEWAY_URL = http://sdcc-gateway-env.eba-ew6wbijd.us-east-1.elasticbeanstalk.com
- Health check path: /healthz
