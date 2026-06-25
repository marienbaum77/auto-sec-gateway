# Auto Sec Gateway

Программно-инфраструктурный комплекс для автоматизированного развертывания и сопровождения отказоустойчивого облачного сетевого шлюза. Проект обеспечивает создание защищенного интернет-канала на базе протокола QUIC с автоматизацией жизненного цикла серверов и пользователей.

## Архитектура и стек технологий

Проект спроектирован по принципам Infrastructure as Code (IaC) и GitOps. Состояние инфраструктуры описано декларативно.

*   **Транспортный уровень:** Hysteria 2 (QUIC / UDP) с алгоритмом обфускации Salamander.
*   **Оркестрация:** K3s (Lightweight Kubernetes).
*   **Управляющий сервис:** Микросервис на языке Go (Stateless Architecture, GORM).
*   **База данных:** PostgreSQL 15.
*   **Сетевой стек:** Ingress Nginx, Cert-Manager (Let's Encrypt).
*   **Автоматизация (IaC & CI/CD):** Ansible, FluxCD, GitHub Actions.
*   **Управление секретами:** Mozilla SOPS + Age.

## Требования к окружению

Для развертывания шлюза потребуется:
1. Виртуальный выделенный сервер (VPS) с установленной ОС Ubuntu 22.04 или 24.04 (рекомендуемые системные требования: 2 vCPU, 2 GB RAM).
2. Зарегистрированное доменное имя, направленное на публичный IP-адрес сервера (A-запись).
3. Токен Telegram-бота (полученный через BotFather).
4. Локальная рабочая станция с Linux (или WSL) с установленными утилитами: `ansible`, `kubectl`, `flux`, `sops`, `age`.

## Подготовка к развертыванию

### 1. Конфигурация параметров среды (Unified Configuration)
Все переменные среды вынесены в единую точку конфигурации. Откройте файл `clusters/my-cluster/infrastructure.yaml` и укажите параметры вашего сервера в блоке `postBuild.substitute`:

```yaml
  postBuild:
    substitute:
      DOMAIN: "your-domain.com"
      PUBLIC_IP: "111.222.333.444"
      EMAIL: "your-email@example.com"
      POSTGRES_USER: "manager"
      POSTGRES_DB: "manager"
```

### 2. Управление секретами (SOPS)
Генерация ключей и шифрование паролей:

1. Сгенерируйте приватный ключ Age на локальной машине:
   ```bash
   age-keygen -o ~/.config/sops/age/keys.txt
   ```
2. Скопируйте полученный публичный ключ (начинается с `age1...`) и вставьте его в файл `.sops.yaml` в корне проекта.
3. Создайте файл `k8s/overlays/prod/secrets.yaml` и укажите в нем открытые пароли:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: app-secrets
     namespace: prod
   stringData:
     postgres-password: "your-db-password"
     hy-password: "your-hysteria-password"
   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: telegram-secret
     namespace: prod
   stringData:
     TELEGRAM_TOKEN: "your-bot-token"
   ```
4. Зашифруйте файл с помощью SOPS:
   ```bash
   sops -e -i k8s/overlays/prod/secrets.yaml
   ```
Зафиксируйте изменения и отправьте их в ваш форк репозитория (`git push`).

## Развертывание инфраструктуры

### 1. Подготовка сервера (Ansible)
Укажите IP-адрес вашего сервера и путь к SSH-ключу в файле `ansible/inventory.ini`. Затем выполните настройку ОС и установку K3s:

```bash
ansible-playbook -i ansible/inventory.ini ansible/setup.yml
ansible-playbook -i ansible/inventory.ini ansible/k3s_install.yml
```
После выполнения скриптов файл доступа к кластеру (`config-remote`) будет автоматически загружен на вашу локальную машину в директорию `~/.kube/`.

### 2. Инициализация GitOps (FluxCD)
Настройте локальный доступ к кластеру:
```bash
export KUBECONFIG=~/.kube/config-remote
```
Запустите контроллер непрерывной доставки FluxCD, указав данные вашего GitHub-репозитория и токен (Personal Access Token):
```bash
flux bootstrap github \
  --owner=Ваш_Пользователь_GitHub \
  --repository=auto-sec-gateway \
  --branch=main \
  --path=./clusters/my-cluster \
  --personal
```
Контроллер автоматически расшифрует секреты, сгенерирует ConfigMaps и развернет все компоненты шлюза. Проверить статус развертывания можно командой: `kubectl get pods -n prod`.

## Управление системой

Взаимодействие с системой осуществляется через Telegram-бота. 

### Назначение первого администратора
Поскольку архитектура системы не использует хранение прав доступа в файлах конфигурации, выдача прав первому администратору производится на уровне базы данных:

1. Отправьте команду `/start` вашему боту в Telegram для первичной регистрации.
2. Подключитесь к базе данных внутри кластера:
   ```bash
   kubectl exec -it deployment/postgres -n prod -- psql -U manager -d manager
   ```
3. Выполните SQL-запрос для привязки роли администратора к вашему ID:
   ```sql
   INSERT INTO administrators (user_id) VALUES (1);
   ```

### Доступные команды администратора
*   `/users` — вывод полного списка зарегистрированных пользователей и их статусов.
*   `/info <telegram_id>` — получение детализированной информации о пользователе (включая UUID и индивидуальную ссылку подписки).
*   `/create <username> <telegram_id>` — принудительная регистрация пользователя (генерация токенов доступа).
*   `/update_status <telegram_id> <true/false>` — активация или деактивация учетной записи пользователя.
*   `/ban <telegram_id>` — быстрая блокировка пользователя с отзывом сетевого доступа.

### Фоновый мониторинг
В управляющий микросервис встроена служба `HealthMonitor`. При недоступности прокси-узла на порту 443 или его восстановлении, администраторы системы автоматически получают диагностические оповещения в Telegram. Управление подписками синхронизировано со статусом узлов в БД.
