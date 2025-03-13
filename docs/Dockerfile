FROM sphinxdoc/sphinx:5.3.0

RUN apt-get update && apt-get install -y wget git

# Note: Any golang version that can 'go list -m -f {{.Variable}}' is fine...
ADD https://go.dev/dl/go1.24.1.linux-amd64.tar.gz /

RUN tar -C /usr/local -xzf /go1.24.1.linux-amd64.tar.gz && \
    rm /go1*.linux-amd64.tar.gz

ENV PATH=$PATH:/usr/local/go/bin

COPY requirements.txt .

RUN pip3 install -r requirements.txt
