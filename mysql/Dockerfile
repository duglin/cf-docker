FROM ubuntu:saucy
RUN apt-get -y update && apt-get -y upgrade
RUN apt-get -y install mysql-server
RUN apt-get -y install mysql-client

ADD my.cnf /etc/mysql/conf.d/my.cnf

EXPOSE 3306

# Use CMD instead of ENTRYPOINT so we can override it easily to either
# debug (via bash) or to pass in default DB/user/password args
CMD mysqld_safe
