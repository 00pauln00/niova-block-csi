module github.com/niova-block-csi

go 1.24.0

toolchain go1.24.6

replace github.com/00pauln00/niova-mdsvc => ./niova-mdsvc

replace github.com/00pauln00/niova-pumicedb/go => ./niova-pumicedb/go

require (
	github.com/00pauln00/niova-mdsvc v0.0.0-20250828071051-291fb9f7747f
	github.com/container-storage-interface/spec v1.8.0
	github.com/google/uuid v1.6.0
	google.golang.org/grpc v1.58.0
	k8s.io/klog/v2 v2.100.1
	k8s.io/mount-utils v0.28.0
)

require (
	github.com/00pauln00/niova-pumicedb/go v0.0.0-20250825081145-6cf0dcb3bbb9  // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-metrics v0.5.4 // indirect
	github.com/hashicorp/go-msgpack v0.5.3 // indirect
	github.com/hashicorp/go-msgpack/v2 v2.1.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-sockaddr v1.0.5 // indirect
	github.com/hashicorp/golang-lru v1.0.2 // indirect
	github.com/hashicorp/logutils v1.0.0 // indirect
	github.com/hashicorp/memberlist v0.5.2 // indirect
	github.com/hashicorp/serf v0.9.7 // indirect
	github.com/miekg/dns v1.1.56 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/sean-/seed v0.0.0-20170313163322-e2103e2c3529 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.40.0 // indirect
	golang.org/x/sync v0.14.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.25.0 // indirect
	golang.org/x/tools v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
)
