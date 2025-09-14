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

var _ = Describe("Database Controller", func() {
	var (
		ctx                context.Context
		database           *dbv1.Database
		datastore          *dbv1.Datastore
		secret             *corev1.Secret
		databaseName       string
		datastoreName      string
		secretName         string
		namespace          string
		typeNamespacedName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		databaseName = "test-database"
		datastoreName = "test-datastore"
		secretName = "test-secret"
		typeNamespacedName = types.NamespacedName{
			Name:      databaseName,
			Namespace: namespace,
		}
	})

	AfterEach(func() {
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

	Context("When creating a Database with ready datastore", func() {
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

			By("Creating a Database resource")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
					Charset:   "UTF8",
					Collation: "en_US.UTF-8",
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())
		})

		It("should add finalizer and update status to Creating", func() {
			By("Reconciling the Database")
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is added")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, database)
				return err == nil && len(database.Finalizers) > 0
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, database)
				return err == nil && database.Status.Phase == "Creating"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("should handle database deletion with finalizer", func() {
			By("Adding finalizer manually")
			database.Finalizers = append(database.Finalizers, databaseFinalizer)
			Expect(k8sClient.Update(ctx, database)).To(Succeed())

			By("Deleting the Database")
			Expect(k8sClient.Delete(ctx, database)).To(Succeed())

			By("Reconciling the Database")
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is removed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, database)
				return err != nil && errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a Database with non-ready datastore", func() {
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

			By("Creating a non-ready Datastore")
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
					Phase: "Connecting",
					Ready: false,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a Database resource")
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
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())
		})

		It("should wait for datastore to be ready", func() {
			By("Reconciling the Database")
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows waiting")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, database)
				return err == nil && database.Status.Phase == "Waiting"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a Database with missing datastore", func() {
		BeforeEach(func() {
			By("Creating a Database resource with non-existent datastore")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: "non-existent-datastore",
					},
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())
		})

		It("should handle missing datastore gracefully", func() {
			By("Reconciling the Database")
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows failure")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, database)
				return err == nil && database.Status.Phase == "Failed"
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
		})

		It("should handle MySQL database creation", func() {
			By("Creating a MySQL Database")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
					Charset:   "utf8mb4",
					Collation: "utf8mb4_unicode_ci",
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			By("Reconciling the Database")
			controllerReconciler := &DatabaseReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, database)
				return err == nil && database.Status.Phase == "Creating"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When handling non-existent resources", func() {
		It("should return without error for non-existent database", func() {
			By("Reconciling a non-existent database")
			controllerReconciler := &DatabaseReconciler{
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
			connStr := buildDatabaseConnectionString("postgres", "testdb", secretData)
			Expect(connStr).To(ContainSubstring("postgres://testuser:testpass@localhost:5432/testdb"))

			By("Testing MySQL connection string")
			secretData["port"] = []byte("3306")
			connStr = buildDatabaseConnectionString("mysql", "testdb", secretData)
			Expect(connStr).To(ContainSubstring("testuser:testpass@tcp(localhost:3306)/testdb"))

			By("Testing SQL Server connection string")
			secretData["port"] = []byte("1433")
			secretData["instance"] = []byte("SQLEXPRESS")
			connStr = buildDatabaseConnectionString("sqlserver", "testdb", secretData)
			Expect(connStr).To(ContainSubstring("sqlserver://testuser:testpass@localhost:1433SQLEXPRESS?database=testdb"))
		})
	})
})
