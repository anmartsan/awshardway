#Create stack

aws cloudformation create-stack --stack-name webapp --template-body file://webapp.yaml

#Update Stack

aws cloudformation update-stack --stack-name webapp --template-body file://webapp.yaml

