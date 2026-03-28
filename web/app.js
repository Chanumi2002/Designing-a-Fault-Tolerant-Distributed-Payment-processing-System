const currentLeader = document.getElementById("currentLeader");
const paymentsTable = document.getElementById("paymentsTable");
const logsPanel = document.getElementById("logsPanel");
const paymentForm = document.getElementById("paymentForm");
const formMessage = document.getElementById("formMessage");

const nodeCards = {
  node1: document.getElementById("node1Card"),
  node2: document.getElementById("node2Card"),
  node3: document.getElementById("node3Card"),
};

const nodeRoles = {
  node1: document.getElementById("node1Role"),
  node2: document.getElementById("node2Role"),
  node3: document.getElementById("node3Role"),
};

const nodeStatus = {
  node1: document.getElementById("node1Status"),
  node2: document.getElementById("node2Status"),
  node3: document.getElementById("node3Status"),
};

const nodeAction = {
  node1: document.getElementById("node1Action"),
  node2: document.getElementById("node2Action"),
  node3: document.getElementById("node3Action"),
};

function capitalize(text) {
  if (!text) return "";
  return text.charAt(0).toUpperCase() + text.slice(1);
}

function money(value) {
  return `$${Number(value).toFixed(2)}`;
}

function renderLeader(leader) {
  currentLeader.textContent = leader ? leader.toUpperCase() : "NONE";
}

function renderNodes(nodes, leader) {
  Object.keys(nodeCards).forEach((nodeId) => {
    nodeCards[nodeId].classList.remove("leader", "follower", "failed");
  });

  nodes.forEach((node) => {
    nodeStatus[node.id].textContent = node.status;
    nodeAction[node.id].textContent = node.lastAction;
    nodeRoles[node.id].textContent = node.role;

    if (node.status === "Failed") {
      nodeCards[node.id].classList.add("failed");
      nodeRoles[node.id].className = "badge warning-badge";
      return;
    }

    if (node.id === leader) {
      nodeCards[node.id].classList.add("leader");
      nodeRoles[node.id].className = "badge leader-badge";
    } else {
      nodeCards[node.id].classList.add("follower");
      nodeRoles[node.id].className = "badge follower-badge";
    }
  });

  syncNodeButtons(nodes);
}

function syncNodeButtons(nodes) {
  const nodeMap = {};
  nodes.forEach((node) => {
    nodeMap[node.id] = node;
  });

  document.querySelectorAll(".node-action-btn").forEach((button) => {
    const action = button.dataset.action;
    const nodeId = button.dataset.nodeId;
    const node = nodeMap[nodeId];

    if (!node) {
      button.disabled = true;
      return;
    }

    if (action === "fail") {
      button.disabled = node.status === "Failed";
      return;
    }

    if (action === "rejoin") {
      button.disabled = node.status !== "Failed";
    }
  });
}

function renderPayments(payments) {
  if (!payments.length) {
    paymentsTable.innerHTML = `
      <tr>
        <td colspan="5">No payments yet</td>
      </tr>
    `;
    return;
  }

  paymentsTable.innerHTML = payments
    .map(
      (payment) => `
        <tr>
          <td>${payment.TransactionID}</td>
          <td>${money(payment.Amount)}</td>
          <td>${payment.OwnerID}</td>
          <td><span class="status ${payment.Status.toLowerCase()}">${capitalize(payment.Status)}</span></td>
          <td>${payment.Version}</td>
        </tr>
      `
    )
    .join("");
}

function renderLogs(logs) {
  if (!logs.length) {
    logsPanel.innerHTML = `<div class="log-line">No logs yet</div>`;
    return;
  }

  logsPanel.innerHTML = logs
    .map((log) => `<div class="log-line">[${log.time}] ${log.message}</div>`)
    .join("");

  logsPanel.scrollTop = logsPanel.scrollHeight;
}

function renderStats(data) {
  document.getElementById("nodesActive").textContent = data.nodesActive;
  document.getElementById("committedCount").textContent = data.committed;
  document.getElementById("pendingCount").textContent = data.pending;
  document.getElementById("recoveryState").textContent = data.recoveryState;
}

async function loadDashboard() {
  try {
    const res = await fetch("/api/dashboard");
    const data = await res.json();

    renderLeader(data.leader);
    renderNodes(data.nodes, data.leader);
    renderPayments(data.payments);
    renderLogs(data.logs);
    renderStats(data);
  } catch (err) {
    formMessage.textContent = "Failed to load dashboard data.";
  }
}

async function failNode(nodeId) {
  try {
    const res = await fetch("/api/fail-node", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ nodeId }),
    });

    const data = await res.json();
    formMessage.textContent = data.message || `${nodeId} failed.`;
    await loadDashboard();
  } catch (err) {
    formMessage.textContent = "Failed to fail node.";
  }
}

async function rejoinNode(nodeId) {
  try {
    const res = await fetch("/api/rejoin-node", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ nodeId }),
    });

    const data = await res.json();
    formMessage.textContent = data.message || `${nodeId} rejoined.`;
    await loadDashboard();
  } catch (err) {
    formMessage.textContent = "Failed to rejoin node.";
  }
}

paymentForm.addEventListener("submit", async (e) => {
  e.preventDefault();

  const transactionId = document.getElementById("transactionId").value.trim();
  const amount = document.getElementById("amount").value.trim();
  const ownerId = document.getElementById("ownerId").value.trim();

  if (!transactionId || !amount || !ownerId) {
    formMessage.textContent = "Please fill all fields.";
    return;
  }

  try {
    const res = await fetch("/api/payments", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        transactionId,
        amount: Number(amount),
        ownerId,
      }),
    });

    const data = await res.json();
    formMessage.textContent = data.message || "Request completed.";

    if (res.ok) {
      paymentForm.reset();
      await loadDashboard();
    }
  } catch (err) {
    formMessage.textContent = "Failed to create payment.";
  }
});

document.querySelectorAll(".node-action-btn").forEach((button) => {
  button.addEventListener("click", async () => {
    const action = button.dataset.action;
    const nodeId = button.dataset.nodeId;

    if (!nodeId || button.disabled) {
      return;
    }

    if (action === "fail") {
      await failNode(nodeId);
      return;
    }

    if (action === "rejoin") {
      await rejoinNode(nodeId);
    }
  });
});

document.getElementById("refreshBtn").addEventListener("click", async () => {
  formMessage.textContent = "Dashboard refreshed.";
  await loadDashboard();
});

loadDashboard();
setInterval(loadDashboard, 3000);