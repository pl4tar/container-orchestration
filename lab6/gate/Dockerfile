FROM python:3.13-alpine
COPY gate.py /gate.py
USER 65534
EXPOSE 9100
ENTRYPOINT ["python", "-u", "/gate.py"]
