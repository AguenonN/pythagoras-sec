# Detective : page en lecture seule sur le journal versé dans PostgreSQL.
# Aiven Runtime injecte DATABASE_URL par l'intégration de service.
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app.py .
RUN useradd --system --uid 10001 detective
USER 10001
ENV PORT=8080
EXPOSE 8080
CMD ["python", "app.py"]
