FROM golang:1.13

RUN mkdir /android-ndk
WORKDIR /android-ndk
RUN curl https://dl.google.com/android/repository/android-ndk-r20-linux-x86_64.zip >> ndk.zip
RUN apt-get update
RUN apt-get install -y unzip
RUN unzip ndk.zip
ENV ANDROID_NDK_HOME /android-ndk/android-ndk-r20

RUN apt-get install -y libegl1-mesa-dev libgles2-mesa-dev libx11-dev
RUN go get -u golang.org/x/mobile/cmd/gomobile

ARG DIR
RUN mkdir -p $DIR
WORKDIR $DIR

ADD go.sum go.mod ./
RUN go mod download

ADD . .
RUN make all
