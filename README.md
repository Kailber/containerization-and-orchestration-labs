# Containerization & Orchestration Labs

Учебный репозиторий команды из 3 студентов для выполнения лабораторных работ по контейнеризации и оркестрации.

## 👥 Команда

| Участник  | GitHub                                   | Telegram |
| --------- | ---------------------------------------- | ---------------------------------------- |
| Student 1 | [Kailber](https://github.com/Kailber)   | [@beverss](https://t.me/beverss)         |
| Student 2 | [12262004-m](https://github.com/12262004-m) | [@MpAsSgHA](https://t.me/MpAsSgHA) |
| Student 3 | [Diminasss](https://github.com/Diminasss) | [@diminass](https://t.me/diminass) |

## 📚 Лабораторные работы

| №  | Тема                  | Статус         |
| -- | --------------------- | -------------- |
| 01 | Свой Docker           | 🟡 In progress  |
| 02 | Мониторинг сервиса: метрики, логи, трейсы        | ⚪ Not started  |

### Статусы

* 🟢 Done
* 🟡 In progress
* ⚪ Not started

## 🛠️ Технологии

* Docker
* Kubernetes
* Git / GitHub

## 📁 Структура проекта

```text
.
├── lab1/
├── lab2/
├── lab3/
├── lab4/
├── lab5/
└── README.md
```

Каждая лабораторная работа находится в отдельной директории и содержит собственный `README.md` с инструкцией по запуску и описанием решения.

## 🚀 Как работать с репозиторием

Клонировать репозиторий:

```bash
git clone https://github.com/Kailber/containerization-and-orchestration-labs.git
cd containerization-and-orchestration-labs
```

Перед началом работы:

```bash
git pull
```

После внесения изменений:

```bash
git add .
git commit -m "lab01: add Dockerfile"
git push
```

## 🌿 Git workflow

Для каждой лабораторной рекомендуется создавать отдельную ветку:

```text
main
 ├── lab1
 ├── lab2
 ├── lab3
 └── lab4
```

Пример:

```bash
git checkout -b lab1
```

После завершения работы создаётся Pull Request в `main`.

## 📝 Правила коммитов

Рекомендуемый формат:

```text
lab01: add Dockerfile
lab01: configure container networking
lab02: add docker compose
lab03: fix healthcheck
lab04: add Kubernetes deployment
```

## 🔀 Pull Requests

Перед merge в `main` желательно:

1. Убедиться, что лабораторная запускается.
2. Проверить README лабораторной.
3. Удалить временные файлы и секреты.
4. Убедиться, что `git status` не содержит лишних файлов.

## 🔐 Secrets

Не коммитить в репозиторий:

* `.env`
* пароли
* API keys
* SSH keys
* kubeconfig
* credentials
* приватные сертификаты

## 📖 Лабораторные

Подробная информация о каждой лабораторной находится в соответствующей директории:

* [Lab 1](./lab1/)
