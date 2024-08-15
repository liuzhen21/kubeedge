#!/bin/bash

# 配置参数
DEPLOYMENT_NAME="hollow-edge-node"
REPLICAS=1
CONTAINER_COUNT=5
IMAGE="harbor.dev.thingsdao.com/edgewize/edgemarks:v1.13.0"
HTTP_SERVER="https://172.31.19.37:30272"
WEBSOCKET_SERVER="172.31.19.37:30271"
SECRET_NAME="tokensecret"
SECRET_KEY="tokendata"
CPU_REQUEST="10m"
MEMORY_REQUEST="50Mi"
PRIVILEGED=true

# 创建 YAML 文件
cat <<EOF > deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: $DEPLOYMENT_NAME
spec:
  replicas: $REPLICAS
  selector:
    matchLabels:
      app: $DEPLOYMENT_NAME
  template:
    metadata:
      labels:
        app: $DEPLOYMENT_NAME
    spec:
      containers:
EOF

# 生成容器配置
for ((i=1; i<=CONTAINER_COUNT; i++))
do
cat <<EOF >> deployment.yaml
        - name: ${DEPLOYMENT_NAME}-${i}
          image: $IMAGE
          command:
            - edgemark
          args:
            - --token=\$(TOKEN)
            - --name=\$(NODE_NAME)-${i}
            - --http-server=$HTTP_SERVER
            - --websocket-server=$WEBSOCKET_SERVER
          env:
            - name: NODE_NAME
              valueFrom:
                fieldRef:
                  apiVersion: v1
                  fieldPath: metadata.name
            - name: TOKEN
              valueFrom:
                secretKeyRef:
                  name: $SECRET_NAME
                  key: $SECRET_KEY
          resources:
            requests:
              cpu: $CPU_REQUEST
              memory: $MEMORY_REQUEST
          securityContext:
            privileged: $PRIVILEGED
EOF
done

# 添加 tolerations 配置
cat <<EOF >> deployment.yaml
      tolerations:
        - effect: NoExecute
          key: node.kubernetes.io/unreachable
          operator: Exists
        - effect: NoExecute
          key: node.kubernetes.io/not-ready
          operator: Exists
EOF

echo "Deployment YAML 文件已生成: deployment.yaml"

