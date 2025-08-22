/*
Copyright 2024 The KubeEdge Authors.

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
package certificate

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/emicklei/go-restful"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog/v2"

	hubconfig "github.com/kubeedge/kubeedge/cloud/pkg/cloudhub/config"
	"github.com/kubeedge/kubeedge/cloud/pkg/cloudhub/servers/httpserver/resps"
	"github.com/kubeedge/kubeedge/common/constants"
	"github.com/kubeedge/kubeedge/common/types"
	"github.com/kubeedge/kubeedge/pkg/security/certs"
	"github.com/kubeedge/kubeedge/pkg/security/token"
)

// GetCA returns the caCertDER
func GetCA(_ *restful.Request, response *restful.Response) {
	resps.OK(response, hubconfig.Config.Ca)
}

// EdgeCoreClientCert will verify the certificate of EdgeCore or token then create EdgeCoreCert and return it
func EdgeCoreClientCert(request *restful.Request, response *restful.Response) {
	r := request.Request
	nodeName := r.Header.Get(types.HeaderNodeName)

	if cert := r.TLS.PeerCertificates; len(cert) > 0 {
		if err := verifyCert(cert[0], nodeName); err != nil {
			message := fmt.Sprintf("failed to verify the certificate for edgenode: %s, err: %v", nodeName, err)
			klog.Error(message)
			resps.ErrorMessage(response, http.StatusUnauthorized, message)
			return
		}
	} else {
		authorization := r.Header.Get(types.HeaderAuthorization)
		if code, err := verifyAuthorization(authorization); err != nil {
			klog.Error(err)
			resps.Error(response, code, err)
			return
		}
	}

	usagesStr := r.Header.Get(types.HeaderExtKeyUsages)
	reader := http.MaxBytesReader(response, r.Body, constants.MaxRespBodyLength)
	certBlock, err := signEdgeCert(reader, usagesStr)
	if err != nil {
		message := fmt.Sprintf("failed to sign certs for edgenode %s, err: %v", nodeName, err)
		klog.Error(message)
		resps.ErrorMessage(response, http.StatusInternalServerError, message)
		return
	}
	resps.OK(response, certBlock.Bytes)
}

// verifyCert verifies the edge certificate by CA certificate when edge certificates rotate.
func verifyCert(cert *x509.Certificate, nodeName string) error {
	roots := x509.NewCertPool()
	ok := roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{
		Type:  certutil.CertificateBlockType,
		Bytes: hubconfig.Config.Ca,
	}))
	if !ok {
		return fmt.Errorf("failed to parse root certificate")
	}
	opts := x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	if _, err := cert.Verify(opts); err != nil {
		return fmt.Errorf("failed to verify edge certificate: %v", err)
	}
	return verifyCertSubject(cert, nodeName)
}

// verifyCertSubject ...
func verifyCertSubject(cert *x509.Certificate, nodeName string) error {
	if cert.Subject.Organization[0] == "KubeEdge" && cert.Subject.CommonName == "kubeedge.io" {
		// In order to maintain compatibility with older versions of certificates
		// this condition will be removed in KubeEdge v1.18.
		return nil
	}
	commonName := fmt.Sprintf("system:node:%s", nodeName)
	if cert.Subject.Organization[0] == "system:nodes" && strings.HasPrefix(cert.Subject.CommonName, commonName) {
		return nil
	}
	return fmt.Errorf("request node name is not match with the certificate")
}

// verifyAuthorization verifies the token from EdgeCore CSR
func verifyAuthorization(authorization string) (int, error) {
	klog.V(4).Info("authorization token is: ", authorization)
	if authorization == "" {
		return http.StatusUnauthorized, errors.New("token validation failure, token is empty")
	}
	bearerToken := strings.Split(authorization, " ")
	if len(bearerToken) != 2 {
		return http.StatusUnauthorized, errors.New("token validation failure, token cannot be splited")
	}
	valid, err := token.Verify(bearerToken[1], hubconfig.Config.CaKey)
	if err != nil {
		return http.StatusUnauthorized, fmt.Errorf("token validation failure, err: %v", err)
	}
	if !valid {
		return http.StatusUnauthorized, errors.New("token validation failure, valid is false")
	}
	return http.StatusOK, nil
}

// signEdgeCert signs the CSR from EdgeCore
func signEdgeCert(r io.ReadCloser, usagesStr string) (*pem.Block, error) {
	klog.V(4).Infof("receive sign crt request, ExtKeyUsages: %s", usagesStr)
	var usages []x509.ExtKeyUsage
	if usagesStr == "" {
		usages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		err := json.Unmarshal([]byte(usagesStr), &usages)
		if err != nil {
			return nil, fmt.Errorf("unmarshal http header ExtKeyUsages fail, err: %v", err)
		}
	}
	payload, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("fail to read file when signing the cert, err: %v", err)
	}
	edgeCertSigningDuration := hubconfig.Config.CloudHub.EdgeCertSigningDuration * time.Hour * 24
	h := certs.GetHandler(certs.HandlerTypeX509)
	certBlock, err := h.SignCerts(certs.SignCertsOptionsWithCSR(
		payload,
		hubconfig.Config.Ca,
		hubconfig.Config.CaKey,
		usages,
		edgeCertSigningDuration,
	))
	if err != nil {
		return nil, fmt.Errorf("fail to signCerts, err: %v", err)
	}
	return certBlock, nil
}

func FilterCert(req *restful.Request, resp *restful.Response, chain *restful.FilterChain) {
	// 1. 检查是否已有TLS证书（直接连接时）
	//if req.Request.TLS != nil && len(req.Request.TLS.PeerCertificates) > 0 {
	//	chain.ProcessFilter(req, resp)
	//	return
	//}
	// 2. 尝试从Caddy透传的头部获取证书
	certHeader := req.Request.Header.Get("X-Forwarded-Client-Cert")
	if certHeader == "" {
		// 没有证书头，继续处理（业务层会处理无证书情况）
		chain.ProcessFilter(req, resp)
		return
	}

	klog.Info("into cert filter with cert")

	// 3. 解析证书
	cert, err := parseCertHeader(certHeader)
	if err != nil {
		klog.Errorf("Failed to parse client certificate: %s error: %v", certHeader, err)
		// 解析证书失败，说明当前请求原并没有带上tls证书，因此清空tls，避免服务端使用gateway的证书
		req.Request.TLS = &tls.ConnectionState{}
		chain.ProcessFilter(req, resp)
		return
	}

	// 4. 创建或更新TLS连接状态
	if req.Request.TLS == nil {
		req.Request.TLS = &tls.ConnectionState{}
	}

	// 添加证书到PeerCertificates
	req.Request.TLS.PeerCertificates = []*x509.Certificate{cert}

	// 5. 继续处理请求
	chain.ProcessFilter(req, resp)
}

// 解析Caddy透传的证书头
func parseCertHeader(header string) (*x509.Certificate, error) {
	// Base64解码
	decoded, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return nil, err
	}

	// 尝试解析PEM格式
	var block *pem.Block
	var certData []byte

	// 移除可能的多余字符
	cleanData := strings.ReplaceAll(string(decoded), "\n", "")
	cleanData = strings.ReplaceAll(cleanData, " ", "")
	decoded = []byte(cleanData)

	// PEM格式可能有多个证书块，我们只需要第一个客户端证书
	for len(decoded) > 0 {
		block, decoded = pem.Decode(decoded)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			certData = block.Bytes
			break
		}
	}

	// 如果没有找到PEM块，尝试直接解析DER
	if certData == nil {
		certData = decoded
	}

	// 解析X.509证书
	cert, err := x509.ParseCertificate(certData)
	if err != nil {
		return nil, err
	}

	return cert, nil
}
