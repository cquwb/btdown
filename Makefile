PROJECT_PATH := $(shell pwd)

all:
	cd src/app && go build -o ${PROJECT_PATH}/bin/btdown app/service/cmd
