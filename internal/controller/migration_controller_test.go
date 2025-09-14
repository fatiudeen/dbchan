/*
Copyright 2025.

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

package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

var _ = Describe("Migration Controller", func() {
	var (
		ctx                context.Context
		migration          *dbv1.Migration
		database           *dbv1.Database
		datastore          *dbv1.Datastore
		secret             *corev1.Secret
		migrationName      string
		databaseName       string
		datastoreName      string
		secretName         string
		namespace          string
		typeNamespacedName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		migrationName = "test-migration"
		databaseName = "test-database"
		datastoreName = "test-datastore"
		secretName = "test-secret"
		typeNamespacedName = types.NamespacedName{
			Name:      migrationName,
			Namespace: namespace,
		}
	})

	AfterEach(func() {
		// Cleanup migration
		if migration != nil {
			Expect(k8sClient.Delete(ctx, migration)).To(Succeed())
		}
		// Cleanup database
		if database != nil {
			Expect(k8sClient.Delete(ctx, database)).To(Succeed())
		}
		// Cleanup datastore
		if datastore != nil {
			Expect(k8sClient.Delete(ctx, datastore)).To(Succeed())
		}
		// Cleanup secret
		if secret != nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	Context("When creating a Migration with ready database", func() {
		BeforeEach(func() {
			By("Creating a secret with database credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"sslmode":  []byte("disable"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a ready Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "postgres",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a ready Database")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
				},
				Status: dbv1.DatabaseStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			By("Creating a Migration resource")
			migration = &dbv1.Migration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      migrationName,
					Namespace: namespace,
				},
				Spec: dbv1.MigrationSpec{
					Version:     "001",
					Description: "Create users table",
					DatabaseRef: corev1.LocalObjectReference{
						Name: databaseName,
					},
					SQL: `
						CREATE TABLE users (
							id SERIAL PRIMARY KEY,
							username VARCHAR(255) NOT NULL UNIQUE,
							email VARCHAR(255) NOT NULL UNIQUE,
							created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
						);
					`,
					RollbackSQL: `
						DROP TABLE IF EXISTS users;
					`,
				},
			}
			Expect(k8sClient.Create(ctx, migration)).To(Succeed())
		})

		It("should add finalizer and update status to Pending", func() {
			By("Reconciling the Migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is added")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, migration)
				return err == nil && len(migration.Finalizers) > 0
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, migration)
				return err == nil && migration.Status.Phase == "Pending"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("should handle migration deletion with rollback", func() {
			By("Adding finalizer manually")
			migration.Finalizers = append(migration.Finalizers, migrationFinalizer)
			Expect(k8sClient.Update(ctx, migration)).To(Succeed())

			By("Deleting the Migration")
			Expect(k8sClient.Delete(ctx, migration)).To(Succeed())

			By("Reconciling the Migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is removed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, migration)
				return err != nil && errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("should handle migration deletion without rollback", func() {
			By("Creating a migration without rollback SQL")
			migrationNoRollback := &dbv1.Migration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "migration-no-rollback",
					Namespace: namespace,
				},
				Spec: dbv1.MigrationSpec{
					Version:     "002",
					Description: "Create products table",
					DatabaseRef: corev1.LocalObjectReference{
						Name: databaseName,
					},
					SQL: `
						CREATE TABLE products (
							id SERIAL PRIMARY KEY,
							name VARCHAR(255) NOT NULL,
							price DECIMAL(10,2) NOT NULL
						);
					`,
					// No RollbackSQL
				},
			}
			Expect(k8sClient.Create(ctx, migrationNoRollback)).To(Succeed())

			By("Adding finalizer manually")
			migrationNoRollback.Finalizers = append(migrationNoRollback.Finalizers, migrationFinalizer)
			Expect(k8sClient.Update(ctx, migrationNoRollback)).To(Succeed())

			By("Deleting the Migration")
			Expect(k8sClient.Delete(ctx, migrationNoRollback)).To(Succeed())

			By("Reconciling the Migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "migration-no-rollback",
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is removed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "migration-no-rollback",
					Namespace: namespace,
				}, migrationNoRollback)
				return err != nil && errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a Migration with non-ready database", func() {
		BeforeEach(func() {
			By("Creating a secret with database credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"sslmode":  []byte("disable"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a ready Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "postgres",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a non-ready Database")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
				},
				Status: dbv1.DatabaseStatus{
					Phase: "Creating",
					Ready: false,
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			By("Creating a Migration resource")
			migration = &dbv1.Migration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      migrationName,
					Namespace: namespace,
				},
				Spec: dbv1.MigrationSpec{
					Version:     "001",
					Description: "Create users table",
					DatabaseRef: corev1.LocalObjectReference{
						Name: databaseName,
					},
					SQL: `
						CREATE TABLE users (
							id SERIAL PRIMARY KEY,
							username VARCHAR(255) NOT NULL UNIQUE
						);
					`,
				},
			}
			Expect(k8sClient.Create(ctx, migration)).To(Succeed())
		})

		It("should wait for database to be ready", func() {
			By("Reconciling the Migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows waiting")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, migration)
				return err == nil && migration.Status.Phase == "Waiting"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a Migration with missing database", func() {
		BeforeEach(func() {
			By("Creating a Migration resource with non-existent database")
			migration = &dbv1.Migration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      migrationName,
					Namespace: namespace,
				},
				Spec: dbv1.MigrationSpec{
					Version:     "001",
					Description: "Create users table",
					DatabaseRef: corev1.LocalObjectReference{
						Name: "non-existent-database",
					},
					SQL: `
						CREATE TABLE users (
							id SERIAL PRIMARY KEY,
							username VARCHAR(255) NOT NULL UNIQUE
						);
					`,
				},
			}
			Expect(k8sClient.Create(ctx, migration)).To(Succeed())
		})

		It("should handle missing database gracefully", func() {
			By("Reconciling the Migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows failure")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, migration)
				return err == nil && migration.Status.Phase == "Failed"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When testing different database types", func() {
		BeforeEach(func() {
			By("Creating a secret for MySQL")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("3306"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a ready MySQL Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "mysql",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a ready MySQL Database")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
				},
				Status: dbv1.DatabaseStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())
		})

		It("should handle MySQL migration", func() {
			By("Creating a MySQL Migration")
			migration = &dbv1.Migration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      migrationName,
					Namespace: namespace,
				},
				Spec: dbv1.MigrationSpec{
					Version:     "001",
					Description: "Create users table",
					DatabaseRef: corev1.LocalObjectReference{
						Name: databaseName,
					},
					SQL: `
						CREATE TABLE users (
							id INT AUTO_INCREMENT PRIMARY KEY,
							username VARCHAR(255) NOT NULL UNIQUE,
							email VARCHAR(255) NOT NULL UNIQUE,
							created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
						);
					`,
					RollbackSQL: `
						DROP TABLE IF EXISTS users;
					`,
				},
			}
			Expect(k8sClient.Create(ctx, migration)).To(Succeed())

			By("Reconciling the Migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, migration)
				return err == nil && migration.Status.Phase == "Pending"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When handling non-existent resources", func() {
		It("should return without error for non-existent migration", func() {
			By("Reconciling a non-existent migration")
			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			nonExistentName := types.NamespacedName{
				Name:      "non-existent",
				Namespace: namespace,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: nonExistentName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing checksum calculation", func() {
		It("should calculate correct SHA256 checksum", func() {
			By("Testing checksum calculation")
			sql := "CREATE TABLE test (id INT PRIMARY KEY);"
			expectedHash := sha256.Sum256([]byte(sql))
			expectedChecksum := hex.EncodeToString(expectedHash[:])

			controllerReconciler := &MigrationReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			actualChecksum := controllerReconciler.calculateChecksum(sql)
			Expect(actualChecksum).To(Equal(expectedChecksum))
		})
	})

	Context("When testing connection string building", func() {
		It("should build correct connection strings for different database types", func() {
			secretData := map[string][]byte{
				"username": []byte("testuser"),
				"password": []byte("testpass"),
				"host":     []byte("localhost"),
				"port":     []byte("5432"),
				"sslmode":  []byte("disable"),
			}

			By("Testing PostgreSQL connection string")
			connStr := buildMigrationConnectionString("postgres", "testdb", secretData)
			Expect(connStr).To(ContainSubstring("postgres://testuser:testpass@localhost:5432/testdb"))

			By("Testing MySQL connection string")
			secretData["port"] = []byte("3306")
			connStr = buildMigrationConnectionString("mysql", "testdb", secretData)
			Expect(connStr).To(ContainSubstring("testuser:testpass@tcp(localhost:3306)/testdb"))

			By("Testing SQL Server connection string")
			secretData["port"] = []byte("1433")
			secretData["instance"] = []byte("SQLEXPRESS")
			connStr = buildMigrationConnectionString("sqlserver", "testdb", secretData)
			Expect(connStr).To(ContainSubstring("sqlserver://testuser:testpass@localhost:1433SQLEXPRESS?database=testdb"))
		})
	})
})
