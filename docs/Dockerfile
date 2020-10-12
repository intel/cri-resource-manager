FROM sphinxdoc/sphinx:3.2.1

RUN apt-get update && apt-get install -y git

COPY requirements.txt .

RUN pip3 install -r requirements.txt
