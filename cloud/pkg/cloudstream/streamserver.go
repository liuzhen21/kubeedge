/*
Copyright 2020 The KubeEdge Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloudstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/emicklei/go-restful"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/klog/v2"

	"github.com/kubeedge/kubeedge/cloud/pkg/cloudstream/config"
	"github.com/kubeedge/kubeedge/cloud/pkg/common/client"
	"github.com/kubeedge/kubeedge/common/constants"
	"github.com/kubeedge/kubeedge/pkg/stream/flushwriter"
)

const (
	// Stream server constants
	StreamCertFile = "/etc/kubeedge-stream.crt"
	StreamKeyFile  = "/etc/kubeedge-stream.key"
	CertFileMode   = 0644

	// Certificate constants
	CertificateBlockType = "CERTIFICATE"
	PrivateKeyBlockType  = "PRIVATE KEY"
)

type StreamServer struct {
	// nextMessageID indicates the next message id
	// it starts from 0 , when receive a new apiserver connection and then add 1
	nextMessageID uint64
	container     *restful.Container
	tunnel        *TunnelServer
}

func newStreamServer(t *TunnelServer) *StreamServer {
	return &StreamServer{
		container: restful.NewContainer(),
		tunnel:    t,
	}
}

func (s *StreamServer) installDebugHandler() {
	ws := new(restful.WebService)
	ws.Path("/containerLogs")
	ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
		To(s.getContainerLogs))
	s.container.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/exec")

	ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
		To(s.getExec))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{containerName}").
		To(s.getExec))
	ws.Route(ws.GET("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.getExec))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.getExec))
	s.container.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/attach")
	ws.Route(ws.GET("/{podNamespace}/{podID}/{containerName}").
		To(s.getAttach))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{containerName}").
		To(s.getAttach))
	ws.Route(ws.GET("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.getAttach))
	ws.Route(ws.POST("/{podNamespace}/{podID}/{uid}/{containerName}").
		To(s.getAttach))
	s.container.Add(ws)

	ws = new(restful.WebService)
	ws.Path("/stats")
	ws.Route(ws.GET("").
		To(s.getMetrics))
	ws.Route(ws.GET("/summary").
		To(s.getMetrics))
	ws.Route(ws.GET("/container").
		To(s.getMetrics))
	ws.Route(ws.GET("/{podName}/{containerName}").
		To(s.getMetrics))
	ws.Route(ws.GET("/{namespace}/{podName}/{uid}/{containerName}").
		To(s.getMetrics))
	s.container.Add(ws)

	// metrics api is widely used for Prometheus
	ws = new(restful.WebService)
	ws.Path("/metrics")
	ws.Route(ws.GET("").
		To(s.getMetrics))
	ws.Route(ws.GET("/cadvisor").
		To(s.getMetrics))
	ws.Route(ws.GET("/probes").
		To(s.getMetrics))
	ws.Route(ws.GET("/resource").
		To(s.getMetrics))
	s.container.Add(ws)
}

func (s *StreamServer) getContainerLogs(r *restful.Request, w *restful.Response) {
	var err error
	defer func() {
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			klog.Errorf("Failed to get container logs, err: %v", err)
		}
	}()

	sessionKey, err := s.getSessionKey(r.Request.URL.Path)
	if err != nil {
		err = fmt.Errorf("can not get session key: %v", err)
		return
	}

	session, ok := s.tunnel.getSession(sessionKey)
	if !ok {
		err = fmt.Errorf("can not find %v session ", sessionKey)
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(http.StatusOK)

	if _, ok := w.ResponseWriter.(http.Flusher); !ok {
		err = fmt.Errorf("unable to convert %v into http.Flusher, cannot show logs", reflect.TypeOf(w))
		return
	}
	fw := flushwriter.Wrap(w.ResponseWriter)

	logConnection, err := session.AddAPIServerConnection(s, &ContainerLogsConnection{
		r:            r,
		flush:        fw,
		session:      session,
		ctx:          r.Request.Context(),
		edgePeerStop: make(chan struct{}),
		closeChan:    make(chan bool),
	})
	if err != nil {
		err = fmt.Errorf("add apiServer connection into %s error %v", session.String(), err)
		return
	}

	defer func() {
		if err != nil {
			session.DeleteAPIServerConnection(logConnection)
			klog.Infof("Delete %s from %s", logConnection.String(), session.String())
		}
	}()

	if err = logConnection.Serve(); err != nil {
		err = fmt.Errorf("apiconnection Serve %s in %s error %v",
			logConnection.String(), session.String(), err)
		return
	}
}

func (s *StreamServer) getMetrics(r *restful.Request, w *restful.Response) {
	var err error
	defer func() {
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			klog.Errorf("Failed to get metrics, err: %v", err)
		}
	}()

	sessionKey := strings.Split(r.Request.Host, ":")[0]
	if forwardedURI := r.Request.Header.Get("X-Forwarded-Uri"); forwardedURI != "" {
		if t := strings.Split(forwardedURI, "/"); strings.HasPrefix(forwardedURI, "/api/v1/nodes/") && len(t) > 6 {
			sessionKey = t[4]
			if ip, ok := s.tunnel.getNodeIP(sessionKey); ok {
				r.Request.Host = fmt.Sprintf("%s:%d", ip, constants.ServerPort)
			}
		}
	}
	session, ok := s.tunnel.getSession(sessionKey)
	if !ok {
		err = fmt.Errorf("can not find %v session ", sessionKey)
		return
	}

	w.WriteHeader(http.StatusOK)

	metricsConnection, err := session.AddAPIServerConnection(s, &ContainerMetricsConnection{
		r:            r,
		writer:       w.ResponseWriter,
		session:      session,
		ctx:          r.Request.Context(),
		edgePeerStop: make(chan struct{}),
		closeChan:    make(chan bool),
	})
	if err != nil {
		err = fmt.Errorf("add apiServer connection into %s error %v", session.String(), err)
		return
	}

	defer func() {
		if err != nil {
			session.DeleteAPIServerConnection(metricsConnection)
			klog.Infof("Delete %s from %s", metricsConnection.String(), session.String())
		}
	}()

	if err = metricsConnection.Serve(); err != nil {
		err = fmt.Errorf("apiconnection Serve %s in %s error %v",
			metricsConnection.String(), session.String(), err)
		return
	}
}

func (s *StreamServer) getExec(request *restful.Request, response *restful.Response) {
	var err error
	defer func() {
		if err != nil {
			response.WriteHeader(http.StatusInternalServerError)
			klog.Errorf("Failed to get exec, err: %v", err)
		}
	}()

	sessionKey, err := s.getSessionKey(request.Request.URL.Path)
	if err != nil {
		err = fmt.Errorf("can not get session key: %v", err)
		return
	}
	session, ok := s.tunnel.getSession(sessionKey)
	if !ok {
		err = fmt.Errorf("exec: can not find %v session ", sessionKey)
		return
	}

	if !httpstream.IsUpgradeRequest(request.Request) {
		err = fmt.Errorf("request was not an upgrade")
		return
	}

	// Once the connection is hijacked, the ErrorResponder will no longer work, so
	// hijacking should be the last step in the upgrade.
	requestHijacker, ok := response.ResponseWriter.(http.Hijacker)
	if !ok {
		klog.V(6).Infof("Unable to hijack response writer: %T", response.ResponseWriter)
		err = fmt.Errorf("request connection cannot be hijacked: %T", response.ResponseWriter)
		return
	}

	requestHijackedConn, _, err := requestHijacker.Hijack()
	if err != nil {
		klog.V(6).Infof("Unable to hijack response: %v", err)
		err = fmt.Errorf("error hijacking connection: %v", err)
		return
	}
	defer requestHijackedConn.Close()

	execConnection, err := session.AddAPIServerConnection(s, &ContainerExecConnection{
		r:            request,
		Conn:         requestHijackedConn,
		session:      session,
		ctx:          request.Request.Context(),
		edgePeerStop: make(chan struct{}, 2),
		closeChan:    make(chan bool),
	})

	if err != nil {
		err = fmt.Errorf("add apiServer exec connection into %s error %v", session.String(), err)
		return
	}

	defer func() {
		if err != nil {
			session.DeleteAPIServerConnection(execConnection)
			klog.Infof("Delete %s from %s", execConnection.String(), session.String())
		}
	}()

	if err = execConnection.Serve(); err != nil {
		err = fmt.Errorf("apiconnection Serve %s in %s error %v",
			execConnection.String(), session.String(), err)
		return
	}
}

func (s *StreamServer) getAttach(request *restful.Request, response *restful.Response) {
	var err error
	defer func() {
		if err != nil {
			response.WriteHeader(http.StatusInternalServerError)
			klog.Errorf("Failed to get attach, err: %v", err)
		}
	}()

	sessionKey, err := s.getSessionKey(request.Request.URL.Path)
	if err != nil {
		err = fmt.Errorf("can not get session key: %v", err)
		return
	}
	session, ok := s.tunnel.getSession(sessionKey)
	if !ok {
		err = fmt.Errorf("attach: can not find %v session ", sessionKey)
		return
	}

	if !httpstream.IsUpgradeRequest(request.Request) {
		err = fmt.Errorf("request was not an upgrade")
		return
	}

	// Once the connection is hijacked, the ErrorResponder will no longer work, so
	// hijacking should be the last step in the upgrade.
	requestHijacker, ok := response.ResponseWriter.(http.Hijacker)
	if !ok {
		klog.V(6).Infof("Unable to hijack response writer: %T", response.ResponseWriter)
		err = fmt.Errorf("request connection cannot be hijacked: %T", response.ResponseWriter)
		return
	}

	requestHijackedConn, _, err := requestHijacker.Hijack()
	if err != nil {
		klog.V(6).Infof("Unable to hijack response: %v", err)
		err = fmt.Errorf("error hijacking connection: %v", err)
		return
	}
	defer requestHijackedConn.Close()

	attachConnection, err := session.AddAPIServerConnection(s, &ContainerAttachConnection{
		r:            request,
		Conn:         requestHijackedConn,
		session:      session,
		ctx:          request.Request.Context(),
		edgePeerStop: make(chan struct{}, 2),
		closeChan:    make(chan bool),
	})

	if err != nil {
		err = fmt.Errorf("add apiServer attach connection into %s error %v", session.String(), err)
		return
	}

	defer func() {
		if err != nil {
			session.DeleteAPIServerConnection(attachConnection)
			klog.Infof("Delete %s from %s", attachConnection.String(), session.String())
		}
	}()

	if err = attachConnection.Serve(); err != nil {
		err = fmt.Errorf("apiconnection Serve %s in %s error %v",
			attachConnection.String(), session.String(), err)
		return
	}
}

func (s *StreamServer) getSessionKey(urlPath string) (string, error) {
	// extract pod namespace and pod name from request
	meta := strings.Split(urlPath, "/")
	if len(meta) < 4 {
		return "", fmt.Errorf("can not get pod name from url path: %s", urlPath)
	}
	namespaceName := meta[2]
	podName := meta[3]

	kubeClient := client.GetKubeClient()
	if kubeClient == nil {
		return "", fmt.Errorf("can not get kube client")
	}

	pod, err := kubeClient.CoreV1().Pods(namespaceName).Get(context.Background(), podName, v1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod %s/%s failed: %v", namespaceName, podName, err)
	}
	return pod.Spec.NodeName, nil
}

func (s *StreamServer) Start() {
	s.installDebugHandler()

	pool := x509.NewCertPool()
	data, err := os.ReadFile(config.Config.TLSStreamCAFile)
	if err != nil {
		klog.Exitf("Read tls stream ca file error %v", err)
		return
	}
	pool.AppendCertsFromPEM(data)

	streamServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Config.StreamPort),
		Handler: s.container,
		TLSConfig: &tls.Config{
			ClientCAs: pool,
			// Populate PeerCertificates in requests, but don't reject connections without verified certificates
			ClientAuth: tls.RequestClientCert,
			MinVersion: tls.VersionTLS12,
		},
	}

	streamCertFile := StreamCertFile
	streamKeyFile := StreamKeyFile
	err = s.initStreamCert(streamCertFile, streamKeyFile)
	if err != nil {
		klog.Exitf("Init stream cert error %v", err)
		return
	}

	klog.Infof("Prepare to start stream server ...")
	err = streamServer.ListenAndServeTLS(streamCertFile, streamKeyFile)
	if err != nil {
		klog.Exitf("Start stream server error %v\n", err)
		return
	}
}

// initStreamCert reads the CA certificate and key from the cloudcore secret,
// generates a new server certificate, and writes it to the specified files
func (s *StreamServer) initStreamCert(streamCertFile, streamKeyFile string) error {
	// Get CA certificate and key from cloudcore secret in kubeedge namespace
	caCrt, caKey, err := s.getCACertificateAndKey()
	if err != nil {
		return fmt.Errorf("failed to get CA certificate and key: %v", err)
	}

	// Generate server certificate using the CA
	serverCrt, serverKey, err := SignCloudCoreCert(caCrt, caKey)
	if err != nil {
		return fmt.Errorf("failed to generate server certificate: %v", err)
	}

	// Write certificate and key to files
	if err := s.writeCertificateFiles(streamCertFile, streamKeyFile, serverCrt, serverKey); err != nil {
		return fmt.Errorf("failed to write certificate files: %v", err)
	}

	return nil
}

// getCACertificateAndKey retrieves the CA certificate and key from the cloudcore secret
func (s *StreamServer) getCACertificateAndKey() ([]byte, []byte, error) {
	kubeClient := client.GetKubeClient()
	if kubeClient == nil {
		return nil, nil, fmt.Errorf("can not get kube client")
	}

	cloudhubSecret, err := kubeClient.CoreV1().Secrets("kubeedge").Get(context.Background(), "cloudcore", v1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get ca secret error: %v", err)
	}

	caCrt, err := s.extractPEMData(cloudhubSecret.Data["tlsCA.crt"], "root ca cert")
	if err != nil {
		return nil, nil, err
	}

	caKey, err := s.extractPEMData(cloudhubSecret.Data["tlsCA.key"], "root ca key")
	if err != nil {
		return nil, nil, err
	}

	return caCrt, caKey, nil
}

// extractPEMData extracts and decodes PEM data from secret
func (s *StreamServer) extractPEMData(data []byte, dataType string) ([]byte, error) {
	if data == nil {
		return nil, fmt.Errorf("secret %s is missing", dataType)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM data for %s", dataType)
	}

	return block.Bytes, nil
}

// writeCertificateFiles writes the certificate and key to the specified files
func (s *StreamServer) writeCertificateFiles(certFile, keyFile string, cert, key []byte) error {
	certPem := pem.EncodeToMemory(&pem.Block{Type: CertificateBlockType, Bytes: cert})
	keyPem := pem.EncodeToMemory(&pem.Block{Type: PrivateKeyBlockType, Bytes: key})

	klog.V(3).Infof("server crt: %s", string(certPem))
	klog.V(3).Infof("server key: %s", string(keyPem))

	if err := os.WriteFile(certFile, certPem, CertFileMode); err != nil {
		return err
	}

	if err := os.WriteFile(keyFile, keyPem, CertFileMode); err != nil {
		return err
	}

	return nil
}