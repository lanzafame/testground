#! /bin/bash

REGION=$(curl -s http://169.254.169.254/latest/meta-data/placement/availability-zone | sed 's/[a-z]$//')
TAG=$(aws ec2 describe-instances --region $REGION --instance-ids `curl -s http://169.254.169.254/latest/meta-data/instance-id` | jq -r '.Reservations[0].Instances[0].Tags | from_entries.TG')

if [ -z "$TAG" ]; then
	echo "Could not find TG tag on EC2 instance"
	exit 1
fi

ansible-playbook -i tg-tag.aws_ec2.yml setup-filebeat.yml --extra-vars "tg_hosts=tag_TG_$TAG" --extra-vars "region=$REGION"
