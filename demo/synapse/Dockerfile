FROM docker.io/matrixdotorg/synapse:v1.120.0

ARG UID

# for munging the config it spits out
RUN apt-get update && apt install wget patch -y
RUN wget -O yq "https://github.com/mikefarah/yq/releases/download/v4.35.1/yq_linux_amd64" && chmod u+x yq

ADD compose-start.sh /compose-start.sh
RUN chmod 770 /compose-start.sh
RUN chown $UID /compose-start.sh
RUN chown $UID /yq
ENTRYPOINT [ "/compose-start.sh" ]
