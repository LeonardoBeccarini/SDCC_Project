FROM python:3.11-slim
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
# stai buildando con contesto = dashboard_service, quindi copia TUTTO il contenuto della cartella
COPY . /app/
RUN pip install --no-cache-dir flask requests
ENV GATEWAY_URL="http://gateway.cloud.svc.cluster.local"
EXPOSE 8080
CMD ["python", "/app/app.py"]
