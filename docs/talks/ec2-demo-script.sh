#!/bin/bash
# EC2 Provider Demo Script
# Use this script to follow along with the talk demo section

# ============================================
# Prerequisites
# ============================================
# 1. Access to an OpenShift/ROSA cluster
# 2. kubectl-mtv installed (kubectl mtv --help)
# 3. AWS credentials with EC2 permissions
# 4. At least one stopped EC2 instance to migrate

# ============================================
# Fetch AWS Credentials from Cluster Secret
# ============================================
# ROSA/OCP clusters store AWS credentials in kube-system/aws-creds
# You can use these for the target account, or provide your own for the source

echo "Fetching AWS credentials from cluster secret (kube-system/aws-creds)..."

# Extract credentials from the cluster's AWS secret
CLUSTER_AWS_KEY=$(kubectl get secret aws-creds -n kube-system -o jsonpath='{.data.aws_access_key_id}' 2>/dev/null | base64 -d)
CLUSTER_AWS_SECRET=$(kubectl get secret aws-creds -n kube-system -o jsonpath='{.data.aws_secret_access_key}' 2>/dev/null | base64 -d)

if [ -n "$CLUSTER_AWS_KEY" ]; then
    echo "Found cluster AWS credentials (aws_access_key_id: ${CLUSTER_AWS_KEY:0:8}...)"
else
    echo "Warning: Could not fetch cluster AWS credentials from kube-system/aws-creds"
    echo "You may need to provide credentials manually"
fi

# ============================================
# Configuration - EDIT THESE VALUES
# ============================================
# Use cluster credentials if available, otherwise use provided values
export EC2_KEY="${EC2_KEY:-${CLUSTER_AWS_KEY:-AKIAXXXXXXXXXXXXXXXX}}"
export EC2_SECRET="${EC2_SECRET:-${CLUSTER_AWS_SECRET:-your-secret-key-here}}"
export EC2_REGION="${EC2_REGION:-us-east-1}"
export PROVIDER_NAME="${PROVIDER_NAME:-demo-ec2}"
export TARGET_NAMESPACE="${TARGET_NAMESPACE:-migrated-vms}"
export PLAN_NAME="${PLAN_NAME:-demo-migration}"
# Set this to your EC2 instance ID
export VM_ID="${VM_ID:-i-0abc123def456}"

echo ""
echo "Configuration:"
echo "  EC2_KEY:          ${EC2_KEY:0:8}..."
echo "  EC2_REGION:       $EC2_REGION"
echo "  PROVIDER_NAME:    $PROVIDER_NAME"
echo "  TARGET_NAMESPACE: $TARGET_NAMESPACE"
echo "  VM_ID:            $VM_ID"
echo ""

# ============================================
# Demo Part 1: Provider Creation & Inventory
# ============================================

echo "============================================"
echo "Step 1: Create EC2 Provider"
echo "============================================"
echo ""
echo "Running: kubectl mtv create provider $PROVIDER_NAME --type ec2 \\"
echo "  --ec2-region $EC2_REGION \\"
echo "  --username \$EC2_KEY \\"
echo "  --password \$EC2_SECRET \\"
echo "  --auto-target-credentials"
echo ""
read -p "Press Enter to execute..."

kubectl mtv create provider "$PROVIDER_NAME" --type ec2 \
  --ec2-region "$EC2_REGION" \
  --username "$EC2_KEY" \
  --password "$EC2_SECRET" \
  --auto-target-credentials

echo ""
echo "Waiting for provider to be ready..."
kubectl get provider "$PROVIDER_NAME" -w &
WATCH_PID=$!
sleep 10
kill $WATCH_PID 2>/dev/null

echo ""
echo "============================================"
echo "Step 2: Explore Inventory"
echo "============================================"
echo ""

echo "--- List all EC2 instances ---"
kubectl mtv get inventory ec2-instance "$PROVIDER_NAME"

echo ""
read -p "Press Enter to continue..."

echo "--- Filter stopped instances (ready for migration) ---"
kubectl mtv get inventory ec2-instance "$PROVIDER_NAME" -q "where powerState = 'Off'"

echo ""
read -p "Press Enter to continue..."

echo "--- List EBS volumes ---"
kubectl mtv get inventory ec2-volume "$PROVIDER_NAME"

echo ""
read -p "Press Enter to continue..."

echo "--- List EBS volume types (for storage mapping) ---"
kubectl mtv get inventory ec2-volume-type "$PROVIDER_NAME"

echo ""
read -p "Press Enter to continue..."

echo "--- List networks (Subnets) ---"
kubectl mtv get inventory ec2-network "$PROVIDER_NAME"

# ============================================
# Demo Part 2: Create Migration Plan
# ============================================

echo ""
echo "============================================"
echo "Step 3: Create Migration Plan"
echo "============================================"
echo ""

# Create target namespace if it doesn't exist
kubectl create namespace "$TARGET_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

echo "Running: kubectl mtv create plan $PLAN_NAME \\"
echo "  --source $PROVIDER_NAME \\"
echo "  --target host \\"
echo "  --vms \"$VM_ID\" \\"
echo "  --target-namespace $TARGET_NAMESPACE \\"
echo "  --default-target-network default \\"
echo "  --default-target-storage-class gp3-csi"
echo ""
read -p "Press Enter to execute..."

kubectl mtv create plan "$PLAN_NAME" \
  --source "$PROVIDER_NAME" \
  --target host \
  --vms "$VM_ID" \
  --target-namespace "$TARGET_NAMESPACE" \
  --default-target-network default \
  --default-target-storage-class gp3-csi

echo ""
echo "--- View created plan ---"
kubectl describe plan "$PLAN_NAME"

echo ""
read -p "Press Enter to start the migration..."

# ============================================
# Demo Part 3: Start Migration
# ============================================

echo ""
echo "============================================"
echo "Step 4: Start Migration"
echo "============================================"
echo ""

kubectl mtv start plan "$PLAN_NAME"

echo ""
echo "--- Watch migration progress ---"
echo "(Press Ctrl+C to stop watching)"
kubectl get migration -w

# ============================================
# Demo Part 4: Debugging Commands
# ============================================

echo ""
echo "============================================"
echo "Debugging Commands Reference"
echo "============================================"
echo ""

echo "# Check migration status:"
echo "kubectl get migration"
echo ""

echo "# Detailed plan status:"
echo "kubectl describe plan $PLAN_NAME"
echo ""

echo "# Check PVCs:"
echo "kubectl get pvc -n $TARGET_NAMESPACE"
echo ""

echo "# Check conversion pod logs:"
echo "kubectl logs -n $TARGET_NAMESPACE -l forklift.konveyor.io/plan=$PLAN_NAME -c virt-v2v"
echo ""

echo "# AWS: Find snapshots by tag:"
echo "aws ec2 describe-snapshots --filters \"Name=tag:forklift.konveyor.io/vmID,Values=$VM_ID\""
echo ""

echo "# AWS: Find created volumes by tag:"
echo "aws ec2 describe-volumes --filters \"Name=tag:forklift.konveyor.io/vmID,Values=$VM_ID\""
echo ""

echo "# Check final VM:"
echo "kubectl get vm -n $TARGET_NAMESPACE"
echo ""

echo "# Start the migrated VM:"
echo "kubectl virt start <vm-name> -n $TARGET_NAMESPACE"
echo ""

echo "============================================"
echo "Demo Complete!"
echo "============================================"
