FROM python:3.14.8-alpine3.24

RUN pip install --no-cache-dir prometheus-client==0.26.0

WORKDIR /app
COPY exporter.py .

USER nobody
ENV PYTHONUNBUFFERED=1
EXPOSE 9800
CMD ["python", "exporter.py"]
