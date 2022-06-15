FROM sphinxdoc/sphinx:3.2.1

RUN apt-get update && apt-get install -y wget git

# Note: Any golang version that can 'go list -m -f {{.Variable}}' is fine...
RUN wget https://go.dev/dl/go1.18.3.linux-amd64.tar.gz && \
    tar -C /usr/local -xzf go1.18.3.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin

COPY requirements.txt .

RUN pip3 install -r requirements.txt
