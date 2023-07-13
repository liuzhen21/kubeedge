批量模拟边缘节点，只能注册，不能调度pod
``` bash
/home/ksv/hollow_edgecore  --http-server https://192.168.100.201:31646 --websocket-server 192.168.100.201:30746 --token e2aa3c4279b6c38a5a4d7ba8068017f35ecd0a2e57f795c5da135b49ffb9efb5.eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2ODkyODM1MDl9.n1Ho8YZHa48JIl8vp5hOduIIE_tEqPAk5qU-XybkzDs --count=100 --name=test0713 --v=9
```
增加参数count，创建结果如下
```
test0713-6    Ready      agent,edge                    11s     v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-60   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-61   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-62   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-63   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-64   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-65   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-66   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-67   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-68   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-69   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-7    Ready      agent,edge                    11s     v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-70   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
test0713-71   Ready      agent,edge                    8s      v1.23.15-kubeedge-v0.0.0-master+$Format:%h$
```