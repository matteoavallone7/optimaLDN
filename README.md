# OptimaLDN
## SDCC Project A.Y. 24/25
This is a microservice-based route planning application that leverages Transport for London (TfL) open data to deliver optimal travel recommendations. 
It goes beyond standard routing by factoring in:

+ _Real-time delays and disruptions (via TfL Line Status API);_

+ _Crowding levels to reflect comfort and reliability;_

+ _Dynamic scoring of routes, combining travel time, service quality, and disruptions;_

+ _User personalization, allowing saving preferred routes and recalculating others;_

The system is built on a microservices architecture, with services communicating over RabbitMQ and data stored in InfluxDB Cloud and PostgreSQL. 
It is deployed via Docker, Docker Compose, and AWS EC2, with some functionality running on AWS Lambda.

## Deployment 

### 1) AWS Setup
Firstly, change the AWS credentials file located in the "aws" folder with your own credentials. 
Head to the AWS console, start the Lab and go to the DynamoDB section:
+ Create two new tables needed for the project:
  - the first one called "ActiveRoutes" with userID as partition key;
  - the second one called "ChosenRoutes" with userID as partition key.
Head to the Lambda section and create a new serverless function. After that, you'll need to upload the code to run:
- Start by cross-compiling serverless.go in the src/lambda directory using:
  ```
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap serverless.go
   zip deployment.zip bootstrap
  ```
  this is needed because otherwise Lambda wouldn't be able to recognize and run the code;
- Upload the .zip in aws lambda
- Add these environmental variables:
  ```
  INFLUXDB_URL=https://us-east-1-1.aws.cloud2.influxdata.com
  INFLUXDB_ORG=TrafficMonitoring
  INFLUXDB_TOKEN=sVUXLvak698BAQHpuxKsvvyvwT35wP610W8ic9VNKN9c60VmSZIbwrPm9iK13pNuUzmy3YAoXH6_Vb6Z7tztCQ==
  INFLUXDB_BUCKET=traffic-data
  TFL_API_KEY=3299fcaa4f9446d0ad41668650c2e172
  ```
- Test if it works
- Create an EventBridge trigger that makes the function run every 15 minutes (choose a name of your choice)
- Go to CloudWatch -> Logs -> Group logs -> open the link with the name of this function -> chech the logs to see if it works properly (should log something like 'Successfully wrote x lines on InfluxDB'.)

### 2) EC2 Setup
Head to the EC2 section in AWS and create a new EC2 instance with Amazon Linux, save the .pem file and ssh into it using:
```
ssh -i <file.pem> ec2-user@<Public-IP-EC2>
```
or connect to it via AWS Management Console. Once connected, run the following code to ensure Docker is properly downloaded and ready to be used:
```
sudo yum update -y
sudo yum install -y docker
sudo systemctl enable docker
sudo systemctl start docker
sudo usermod -aG docker $USER
sudo mkdir -p /usr/local/lib/docker/cli-plugins/
sudo curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
  -o /usr/local/lib/docker/cli-plugins/docker-compose
sudo chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
```
Now change directory and clone the repository:
```
git clone https://github.com/matteoavallone7/optimaLDN.git
cd optimaLDN
```

### 3) Docker Compose
Once inside the project directory, just run:
```
docker compose up --build -d
> when finished
docker compose down
```

### 4) Frontend
To run the frontend, first head to main.go file and change the wsURL and baseURL with the EC2 instance public DNS. Then open a terminal locally, cd to the project directory and:
```
cd cmd
go build -o cmd github.com/matteoavallone7/optimaLDN/cmd
./cmd
```

### 5) Test
To run the Recalculate Route test (while the app is running), open a new terminal and cd to the test directory:
```
cd test
go run .
```
